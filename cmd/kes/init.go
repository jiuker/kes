package main

import (
	"fmt"
	"log"
	"net"

	"github.com/charmbracelet/huh"
)

func initCmd(args []string) {
	var ip string
	var dns string
	var generate string
	var prefix string
	var cache string
	var loggingLevel string
	var configuration string
	fmt.Println("(loads config if present, if present the values will be default)")
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Specify IP address you would like to expose KES to?").
				Value(&ip).
				Validate(func(str string) error {
					pip := net.ParseIP(str)
					if pip == nil {
						return fmt.Errorf("unknow ip:%s", str)
					}
					return nil
				}),
			huh.NewInput().
				Title("Specify DNS name the server should use?").
				Value(&dns).
				Validate(func(str string) error {
					return nil
				}),
			huh.NewSelect[string]().
				Title("Would you like KES to generate certificates?").
				Options(
					huh.NewOption("Yes", "yes"),
					huh.NewOption("No", "no"),
				).
				Value(&generate),
			huh.NewInput().
				Title("Specify certificate file name prefix:").
				Value(&prefix).
				Validate(func(str string) error {
					return nil
				}),
			huh.NewSelect[string]().
				Title("Choose your cache configuration:").
				Options(
					huh.NewOption("Liberal (5 minutes)", "A"),
					huh.NewOption("Moderate (1 minute)", "B"),
					huh.NewOption("Conservative (30 seconds)", "C"),
				).
				Value(&cache),
			huh.NewSelect[string]().
				Title("Choose logging level:").
				Options(
					huh.NewOption("Error logging", "A"),
					huh.NewOption("Audit Logging", "B"),
					huh.NewOption("Both", "C"),
				).
				Value(&loggingLevel),
			huh.NewSelect[string]().
				Title("Select KMS Configuration:").
				Options(
					huh.NewOption("Do not persist keys (in-memory only)", "A"),
					huh.NewOption("Hashicorp Vault", "B"),
					huh.NewOption("Fortanix SDKMS", "C"),
					huh.NewOption("Thales CipherTrust Manager / Gemalto KeySecure", "D"),
					huh.NewOption("AWS SecretsManager", "E"),
					huh.NewOption("GCP SecretManager", "F"),
					huh.NewOption("Azure KeyVault", "G"),
					huh.NewOption("File system (testing only)", "H"),
				).
				Value(&configuration),
		),
	)
	err := form.WithAccessible(true).Run()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(ip, dns, generate, prefix, cache, loggingLevel, configuration)
}
