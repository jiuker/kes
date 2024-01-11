// Copyright 2019 - MinIO, Inc. All rights reserved.
// Use of this source code is governed by the AGPLv3
// license that can be found in the LICENSE file.

package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	tui "github.com/charmbracelet/lipgloss"
	"github.com/minio/kes-go"
	"github.com/minio/kes/internal/cli"
	"github.com/minio/kes/internal/https"
	"github.com/minio/kes/internal/sys"
	flag "github.com/spf13/pflag"
	"golang.org/x/term"
)

type commands = map[string]func([]string)

const usage = `Usage:
    kes [options] <command>

Commands:
    server                   Start a KES server.

    key                      Manage cryptographic keys.
    policy                   Manage KES policies.
    identity                 Manage KES identities.

    log                      Print error and audit log events.
    status                   Print server status.
    metric                   Print server metrics.

    migrate                  Migrate KMS data.
    update                   Update KES binary.
    init                     Init with prompts 

Options:
    -v, --version            Print version information.
        --auto-completion    Install auto-completion for this shell.
    -h, --help               Print command line options.
`

func main() {
	if complete(filepath.Base(os.Args[0])) {
		return
	}

	cmd := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	cmd.Usage = func() { fmt.Fprint(os.Stderr, usage) }

	subCmds := commands{
		"server": serverCmd,

		"key":      keyCmd,
		"policy":   policyCmd,
		"identity": identityCmd,

		"log":    logCmd,
		"status": statusCmd,
		"metric": metricCmd,

		"migrate": migrateCmd,
		"update":  updateCmd,
		"init":    initCmd,
	}

	if len(os.Args) < 2 {
		cmd.Usage()
		os.Exit(2)
	}
	if subCmd, ok := subCmds[os.Args[1]]; ok {
		subCmd(os.Args[1:])
		return
	}

	var (
		showVersion    bool
		autoCompletion bool
	)
	cmd.BoolVarP(&showVersion, "version", "v", false, "Print version information.")
	cmd.BoolVar(&autoCompletion, "auto-completion", false, "Install auto-completion for this shell")
	if err := cmd.Parse(os.Args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(2)
		}
		cli.Fatalf("%v. See 'kes --help'", err)

	}
	if cmd.NArg() > 1 {
		cli.Fatalf("%q is not a kes command. See 'kes --help'", cmd.Arg(1))
	}

	if showVersion {
		info, err := sys.ReadBinaryInfo()
		if err != nil {
			cli.Fatal(err)
		}

		faint := tui.NewStyle().Faint(true)
		buf := &strings.Builder{}
		fmt.Fprintf(buf, "Version    %-22s %s\n", info.Version, faint.Render("commit="+info.CommitID))
		fmt.Fprintf(buf, "Runtime    %-22s %s\n", fmt.Sprintf("%s %s/%s", info.Runtime, runtime.GOOS, runtime.GOARCH), faint.Render("compiler="+info.Compiler))
		fmt.Fprintf(buf, "License    %-22s %s\n", "AGPLv3", faint.Render("https://www.gnu.org/licenses/agpl-3.0.html"))
		fmt.Fprintf(buf, "Copyright  %-22s %s\n", fmt.Sprintf("2015-%d MinIO Inc.", time.Now().Year()), faint.Render("https://min.io"))
		fmt.Print(buf.String())
		return
	}

	if autoCompletion {
		installAutoCompletion()
		return
	}

	cmd.Usage()
	os.Exit(2)
}

func newClient(insecureSkipVerify bool) *kes.Client {
	const DefaultServer = "https://127.0.0.1:7373"
	const (
		EnvServer     = "KES_SERVER"
		EnvAPIKey     = "KES_API_KEY"
		EnvClientKey  = "KES_CLIENT_KEY"
		EnvClientCert = "KES_CLIENT_CERT"
	)

	if apiKey, ok := os.LookupEnv(EnvAPIKey); ok {
		if _, ok = os.LookupEnv(EnvClientCert); ok {
			cli.Fatalf("two conflicting environment variables set: unset either '%s' or '%s'", EnvAPIKey, EnvClientCert)
		}
		if _, ok = os.LookupEnv(EnvClientKey); ok {
			cli.Fatalf("two conflicting environment variables set: unset either '%s' or '%s'", EnvAPIKey, EnvClientKey)
		}
		key, err := kes.ParseAPIKey(apiKey)
		if err != nil {
			cli.Fatalf("invalid API key: %v", err)
		}
		cert, err := kes.GenerateCertificate(key)
		if err != nil {
			cli.Fatalf("failed to generate client certificate from API key: %v", err)
		}

		addr := DefaultServer
		if env, ok := os.LookupEnv(EnvServer); ok {
			addr = env
		}
		return kes.NewClientWithConfig(addr, &tls.Config{
			Certificates:       []tls.Certificate{cert},
			InsecureSkipVerify: insecureSkipVerify,
		})
	}

	certPath, ok := os.LookupEnv(EnvClientCert)
	if !ok {
		cli.Fatalf("no TLS client certificate. Environment variable '%s' is not set", EnvClientCert)
	}
	if strings.TrimSpace(certPath) == "" {
		cli.Fatalf("no TLS client certificate. Environment variable '%s' is empty", EnvClientCert)
	}

	keyPath, ok := os.LookupEnv(EnvClientKey)
	if !ok {
		cli.Fatalf("no TLS private key. Environment variable '%s' is not set", EnvClientKey)
	}
	if strings.TrimSpace(keyPath) == "" {
		cli.Fatalf("no TLS private key. Environment variable '%s' is empty", EnvClientKey)
	}

	certPem, err := os.ReadFile(certPath)
	if err != nil {
		cli.Fatalf("failed to load TLS certificate: %v", err)
	}
	certPem, err = https.FilterPEM(certPem, func(b *pem.Block) bool { return b.Type == "CERTIFICATE" })
	if err != nil {
		cli.Fatalf("failed to load TLS certificate: %v", err)
	}
	keyPem, err := os.ReadFile(keyPath)
	if err != nil {
		cli.Fatalf("failed to load TLS private key: %v", err)
	}

	// Check whether the private key is encrypted. If so, ask the user
	// to enter the password on the CLI.
	privateKey, err := decodePrivateKey(keyPem)
	if err != nil {
		cli.Fatalf("failed to read TLS private key: %v", err)
	}
	if len(privateKey.Headers) > 0 && x509.IsEncryptedPEMBlock(privateKey) {
		fmt.Fprint(os.Stderr, "Enter password for private key: ")
		password, err := term.ReadPassword(int(os.Stderr.Fd()))
		if err != nil {
			cli.Fatalf("failed to read private key password: %v", err)
		}
		fmt.Fprintln(os.Stderr) // Add the newline again

		decPrivateKey, err := x509.DecryptPEMBlock(privateKey, password)
		if err != nil {
			if errors.Is(err, x509.IncorrectPasswordError) {
				cli.Fatalf("incorrect password")
			}
			cli.Fatalf("failed to decrypt private key: %v", err)
		}
		keyPem = pem.EncodeToMemory(&pem.Block{Type: privateKey.Type, Bytes: decPrivateKey})
	}

	cert, err := tls.X509KeyPair(certPem, keyPem)
	if err != nil {
		cli.Fatalf("failed to load TLS private key or certificate: %v", err)
	}

	addr := DefaultServer
	if env, ok := os.LookupEnv(EnvServer); ok {
		addr = env
	}
	return kes.NewClientWithConfig(addr, &tls.Config{
		Certificates:       []tls.Certificate{cert},
		InsecureSkipVerify: insecureSkipVerify,
	})
}

func newEnclave(name string, insecureSkipVerify bool) *kes.Enclave {
	client := newClient(insecureSkipVerify)
	if name == "" {
		name = os.Getenv("KES_ENCLAVE")
	}
	return client.Enclave(name)
}

func isTerm(f *os.File) bool { return term.IsTerminal(int(f.Fd())) }

func decodePrivateKey(pemBlock []byte) (*pem.Block, error) {
	ErrNoPrivateKey := errors.New("no PEM-encoded private key found")

	for len(pemBlock) > 0 {
		next, rest := pem.Decode(pemBlock)
		if next == nil {
			return nil, ErrNoPrivateKey
		}
		if next.Type == "PRIVATE KEY" || strings.HasSuffix(next.Type, " PRIVATE KEY") {
			return next, nil
		}
		pemBlock = rest
	}
	return nil, ErrNoPrivateKey
}
