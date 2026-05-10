package email

import (
	"bytes"
	"fmt"
	"text/template"
)

// VerifyEmailData feeds the account-verification template.
// Name is shown in the greeting; VerifyURL is the single-use confirmation link.
type VerifyEmailData struct {
	ToAddress string
	Name      string
	VerifyURL string
}

// PasswordResetEmailData feeds the password-reset template.
// ExpiryMin is the link's lifetime in minutes, surfaced to the user.
type PasswordResetEmailData struct {
	ToAddress string
	Name      string
	ResetURL  string
	ExpiryMin int
}

const verifyEmailSubject = "Confirme seu email - verifique sua conta"

const verifyEmailText = `Olá {{.Name}},

Bem-vinda(o) à nossa loja! Para concluir o cadastro, confirme seu email clicando no link abaixo:

{{.VerifyURL}}

Se você não criou uma conta, pode ignorar esta mensagem.

Atenciosamente,
Equipe da Loja
`

const verifyEmailHTML = `<!doctype html>
<html lang="pt-BR">
<body>
<p>Olá {{.Name}},</p>
<p>Bem-vinda(o) à nossa loja! Para concluir o cadastro, confirme seu email clicando no link abaixo:</p>
<p><a href="{{.VerifyURL}}">Confirmar meu email</a></p>
<p>Ou copie e cole este endereço no navegador: {{.VerifyURL}}</p>
<p>Se você não criou uma conta, pode ignorar esta mensagem.</p>
<p>Atenciosamente,<br>Equipe da Loja</p>
</body>
</html>
`

const passwordResetSubject = "Redefinição de senha"

const passwordResetText = `Olá {{.Name}},

Recebemos um pedido para redefinir sua senha. Clique no link abaixo para criar uma nova senha:

{{.ResetURL}}

Este link expira em {{.ExpiryMin}} minutos. Se você não solicitou a redefinição, ignore esta mensagem.

Atenciosamente,
Equipe da Loja
`

const passwordResetHTML = `<!doctype html>
<html lang="pt-BR">
<body>
<p>Olá {{.Name}},</p>
<p>Recebemos um pedido para redefinir sua senha. Clique no botão abaixo para criar uma nova senha:</p>
<p><a href="{{.ResetURL}}">Redefinir minha senha</a></p>
<p>Ou copie e cole este endereço no navegador: {{.ResetURL}}</p>
<p>Este link expira em {{.ExpiryMin}} minutos. Se você não solicitou a redefinição, ignore esta mensagem.</p>
<p>Atenciosamente,<br>Equipe da Loja</p>
</body>
</html>
`

// RenderVerifyEmail builds the verify-email Message in pt-BR for the given recipient.
func RenderVerifyEmail(data VerifyEmailData) (Message, error) {
	textBody, err := render(verifyEmailText, data)
	if err != nil {
		return Message{}, fmt.Errorf("email: render verify text: %w", err)
	}
	htmlBody, err := render(verifyEmailHTML, data)
	if err != nil {
		return Message{}, fmt.Errorf("email: render verify html: %w", err)
	}
	return Message{
		To:       []string{data.ToAddress},
		Subject:  verifyEmailSubject,
		TextBody: textBody,
		HTMLBody: htmlBody,
		Tags:     map[string]string{"category": "verify_email"},
	}, nil
}

// RenderPasswordResetEmail builds the password-reset Message in pt-BR for the given recipient.
func RenderPasswordResetEmail(data PasswordResetEmailData) (Message, error) {
	textBody, err := render(passwordResetText, data)
	if err != nil {
		return Message{}, fmt.Errorf("email: render reset text: %w", err)
	}
	htmlBody, err := render(passwordResetHTML, data)
	if err != nil {
		return Message{}, fmt.Errorf("email: render reset html: %w", err)
	}
	return Message{
		To:       []string{data.ToAddress},
		Subject:  passwordResetSubject,
		TextBody: textBody,
		HTMLBody: htmlBody,
		Tags:     map[string]string{"category": "password_reset"},
	}, nil
}

func render(tpl string, data any) (string, error) {
	t, err := template.New("email").Parse(tpl)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
