module github.com/89jobrien/devkit

go 1.26.1

require (
	github.com/BurntSushi/toml v1.6.0
	github.com/anthropics/anthropic-sdk-go v1.27.1
	github.com/spf13/cobra v1.10.2
	github.com/stretchr/testify v1.11.1
	golang.org/x/sync v0.16.0
)

require (
	baml_devkit v0.0.0 // indirect
	github.com/boundaryml/baml v0.220.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	github.com/tidwall/gjson v1.18.0 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	google.golang.org/protobuf v1.36.6 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace baml_devkit => ./internal/baml/baml_client
