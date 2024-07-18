module github.com/nyaruka/mailroom

require (
	github.com/Masterminds/semver v1.5.0
	github.com/apex/log v1.1.4
	github.com/aws/aws-sdk-go v1.40.56
	github.com/buger/jsonparser v1.0.0
	github.com/certifi/gocertifi v0.0.0-20200211180108-c7c1fbc02894 // indirect
	github.com/edganiukov/fcm v0.4.0
	github.com/getsentry/raven-go v0.1.2-0.20190125112653-238ebd86338d // indirect
	github.com/go-chi/chi v4.1.2+incompatible
	github.com/golang-jwt/jwt v3.2.2+incompatible
	github.com/golang/protobuf v1.5.2
	github.com/gomodule/redigo v2.0.0+incompatible
	github.com/gorilla/schema v1.1.0
	github.com/jmoiron/sqlx v1.3.4
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/lib/pq v1.10.4
	github.com/nyaruka/ezconf v0.2.1
	github.com/nyaruka/gocommon v1.15.1
	github.com/nyaruka/goflow v0.144.3
	github.com/nyaruka/librato v1.0.0
	github.com/nyaruka/logrus_sentry v0.8.2-0.20190129182604-c2962b80ba7d
	github.com/nyaruka/null v1.2.0
	github.com/nyaruka/redisx v0.1.0
	github.com/olivere/elastic/v7 v7.0.22
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_model v0.3.0
	github.com/prometheus/common v0.37.0
	github.com/shopspring/decimal v1.2.0
	github.com/sirupsen/logrus v1.6.0
	github.com/stretchr/testify v1.7.0
	gopkg.in/go-playground/validator.v9 v9.31.0
)

require (
	github.com/DATA-DOG/go-sqlmock v1.5.0
	github.com/antlr/antlr4 v0.0.0-20200701161529-3d9351f61e0f // indirect
	github.com/blevesearch/segment v0.9.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/fatih/structs v1.0.0 // indirect
	github.com/go-mail/mail v2.3.1+incompatible // indirect
	github.com/go-playground/locales v0.14.0 // indirect
	github.com/go-playground/universal-translator v0.18.0 // indirect
	github.com/gofrs/uuid v3.3.0+incompatible // indirect
	github.com/google/go-querystring v1.1.0
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/konsorten/go-windows-terminal-sequences v1.0.3 // indirect
	github.com/leodido/go-urn v1.2.1 // indirect
	github.com/mailru/easyjson v0.7.6 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.1 // indirect
	github.com/naoina/go-stringutil v0.1.0 // indirect
	github.com/naoina/toml v0.1.1 // indirect
	github.com/nyaruka/phonenumbers v1.0.71 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/sergi/go-diff v1.1.0 // indirect
	golang.org/x/net v0.0.0-20220624214902-1bab6f366d9e // indirect
	golang.org/x/sys v0.0.0-20220520151302-bc2c85ada10a // indirect
	golang.org/x/text v0.3.7 // indirect
	google.golang.org/protobuf v1.28.1 // indirect
	gopkg.in/alexcesaro/quotedprintable.v3 v3.0.0-20150716171945-2caba252f4dc // indirect
	gopkg.in/yaml.v3 v3.0.0-20200313102051-9f266ea9e77c // indirect
)

require (
	github.com/gabriel-vasile/mimetype v1.4.1
	github.com/prometheus/client_golang v1.14.0
)

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.1.2 // indirect
	github.com/prometheus/procfs v0.8.0 // indirect
)

go 1.17

replace github.com/nyaruka/gocommon => github.com/Ilhasoft/gocommon v1.16.2-weni

replace github.com/nyaruka/goflow => github.com/weni-ai/goflow v0.10.0-goflow-0.144.3-4-develop
