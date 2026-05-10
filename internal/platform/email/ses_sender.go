package email

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awssesv2 "github.com/aws/aws-sdk-go-v2/service/sesv2"
	awssesv2types "github.com/aws/aws-sdk-go-v2/service/sesv2/types"
)

// SESConfig configures the SESSender constructor.
type SESConfig struct {
	Region           string
	FromAddress      string
	FromName         string
	ConfigurationSet string
}

// sesAPI is the minimal SES SendEmail surface used here. Implemented by
// awssesv2.Client and by test fakes.
type sesAPI interface {
	SendEmail(ctx context.Context, in *awssesv2.SendEmailInput, optFns ...func(*awssesv2.Options)) (*awssesv2.SendEmailOutput, error)
}

// SESSender delivers messages via AWS SES v2.
type SESSender struct {
	api              sesAPI
	from             string
	configurationSet string
}

var _ Sender = (*SESSender)(nil)

// NewSESSender builds a real SESSender using the AWS default credential chain.
func NewSESSender(cfg SESConfig) (*SESSender, error) {
	if cfg.Region == "" {
		return nil, fmt.Errorf("email: SES region required")
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(cfg.Region),
	)
	if err != nil {
		return nil, fmt.Errorf("email: load aws config: %w", err)
	}
	from := cfg.FromAddress
	if cfg.FromName != "" {
		from = fmt.Sprintf("%s <%s>", cfg.FromName, cfg.FromAddress)
	}
	return &SESSender{
		api:              awssesv2.NewFromConfig(awsCfg),
		from:             from,
		configurationSet: cfg.ConfigurationSet,
	}, nil
}

// NewSESSenderWithAPI is a constructor for tests; injects the SES API surface.
func NewSESSenderWithAPI(api sesAPI, from, configurationSet string) *SESSender {
	return &SESSender{api: api, from: from, configurationSet: configurationSet}
}

// Send delivers msg via SES.
func (s *SESSender) Send(ctx context.Context, msg Message) error {
	in := &awssesv2.SendEmailInput{
		FromEmailAddress: aws.String(s.from),
		Destination: &awssesv2types.Destination{
			ToAddresses: msg.To,
		},
		Content: &awssesv2types.EmailContent{
			Simple: &awssesv2types.Message{
				Subject: &awssesv2types.Content{Data: aws.String(msg.Subject), Charset: aws.String("UTF-8")},
				Body: &awssesv2types.Body{
					Text: &awssesv2types.Content{Data: aws.String(msg.TextBody), Charset: aws.String("UTF-8")},
					Html: &awssesv2types.Content{Data: aws.String(msg.HTMLBody), Charset: aws.String("UTF-8")},
				},
			},
		},
	}
	if s.configurationSet != "" {
		in.ConfigurationSetName = aws.String(s.configurationSet)
	}
	for k, v := range msg.Tags {
		in.EmailTags = append(in.EmailTags, awssesv2types.MessageTag{
			Name:  aws.String(k),
			Value: aws.String(v),
		})
	}

	if _, err := s.api.SendEmail(ctx, in); err != nil {
		return fmt.Errorf("email: ses send: %w", err)
	}
	return nil
}
