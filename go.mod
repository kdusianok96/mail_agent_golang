module go-smtp

go 1.22.2

require (
	github.com/BurntSushi/toml v1.3.2
	github.com/spf13/cobra v1.8.0 // Manually added for CLI
	github.com/spf13/pflag v1.0.5 // Cobra dependency, often good to make it explicit
	github.com/inconshreveable/mousetrap v1.1.0 // Cobra dependency
	golang.org/x/crypto v0.21.0 // Manually added for bcrypt
	github.com/toorop/go-dkim v0.0.0-20230510090721-cff1a93eb758 // Manually added for DKIM signing
)
