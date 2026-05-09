env "local" {
  src = "file://db/migrations"
  url = getenv("DATABASE_URL")
  dev = "docker://postgres/16/dev?search_path=public"

  migration {
    dir = "file://db/migrations"
  }
}
