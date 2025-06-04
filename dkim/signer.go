package dkim

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/toorop/go-dkim"
)

const dkimLogPrefix = "DKIM_SIGNER"

// DKIMSignOptions holds the necessary configuration for signing an email with DKIM.
type DKIMSignOptions struct {
	Domain             string
	Selector           string
	PrivateKeyPath     string
	HeadersToSign      []string // Optional, if empty library defaults or a standard set will be used
	// Canonicalization can be specified if needed, e.g., "relaxed/relaxed"
	// For toorop/go-dkim, the library defaults to "simple/simple".
	// To use relaxed/relaxed, you'd set sigOpts.HeaderC13n = dkim.CanonicalizationRelaxed
	// and sigOpts.BodyC13n = dkim.CanonicalizationRelaxed.
}

// SignEmail attempts to sign the given email data using DKIM.
// It modifies emailData in place by prepending the DKIM-Signature header.
func SignEmail(emailData []byte, opts DKIMSignOptions) ([]byte, error) {
	log.Printf("INFO: %s: Attempting to sign email for domain %s, selector %s", dkimLogPrefix, opts.Domain, opts.Selector)

	privateKeyBytes, err := os.ReadFile(opts.PrivateKeyPath)
	if err != nil {
		log.Printf("ERROR: %s: Failed to read private key from %s: %v", dkimLogPrefix, opts.PrivateKeyPath, err)
		return emailData, fmt.Errorf("failed to read DKIM private key: %w", err)
	}
	log.Printf("INFO: %s: Successfully loaded private key from %s", dkimLogPrefix, opts.PrivateKeyPath)

	sigOpts := dkim.NewSigOptions()
	sigOpts.Domain = opts.Domain
	sigOpts.Selector = opts.Selector
	sigOpts.Algo = "rsa-sha256" // Common algorithm
	sigOpts.Expiration = 0     // No expiration by default, can be set e.g. time.Now().Add(7 * 24 * time.Hour).Unix()
	sigOpts.SignatureTimestamp = true // Include timestamp in signature

	// Set canonicalization. toorop/go-dkim defaults to "simple/simple".
	// For "relaxed/relaxed", which is common and often recommended:
	sigOpts.HeaderC13n = dkim.CanonicalizationRelaxed
	sigOpts.BodyC13n = dkim.CanonicalizationRelaxed
	log.Printf("INFO: %s: Using canonicalization: header=%s, body=%s", dkimLogPrefix, sigOpts.HeaderC13n, sigOpts.BodyC13n)


	// Specify headers to sign.
	// If opts.HeadersToSign is empty, go-dkim uses a default set of headers.
	// Default headers in toorop/go-dkim (as of common versions) usually include:
	// From, Reply-To, Subject, Date, To, Cc, Resent-Date, Resent-From, Resent-To,
	// Resent-Cc, In-Reply-To, References, List-Id, List-Help, List-Unsubscribe,
	// List-Subscribe, List-Post, List-Owner, List-Archive
	if len(opts.HeadersToSign) > 0 {
		hdrs := make([][]byte, len(opts.HeadersToSign))
		for i, h := range opts.HeadersToSign {
			hdrs[i] = []byte(h)
		}
		sigOpts.Headers = hdrs
		log.Printf("INFO: %s: Using custom headers for signing: %v", dkimLogPrefix, opts.HeadersToSign)
	} else {
		log.Printf("INFO: %s: Using library default headers for signing.", dkimLogPrefix)
	}

	// The dkim.Sign function modifies the emailData byte slice in place
	// by prepending the DKIM-Signature header.
	// It expects emailData to be in RFC5322 format (CRLF line endings).
	err = dkim.Sign(&emailData, privateKeyBytes, sigOpts)
	if err != nil {
		log.Printf("ERROR: %s: Failed to sign email: %v", dkimLogPrefix, err)
		return emailData, fmt.Errorf("DKIM signing failed: %w", err)
	}

	log.Printf("INFO: %s: Email successfully signed for domain %s, selector %s.", dkimLogPrefix, opts.Domain, opts.Selector)
	return emailData, nil
}
