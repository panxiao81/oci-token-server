// Command oci-token-server issues registry bearer tokens following the
// distribution token authentication spec
// (https://distribution.github.io/distribution/spec/auth/token/).
//
// It currently grants anonymous pull only: every repository scope in the
// request is narrowed to "pull", every other scope is dropped, and credentials
// are ignored. Pair it with a registry that already authenticates pushers
// itself, e.g. zot with bearer.oidc plus upstreamTokenEndpoint pointing here.
package main

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type access struct {
	Type    string   `json:"type"`
	Name    string   `json:"name"`
	Actions []string `json:"actions"`
}

// pullScopes narrows requested scopes to repository:<name>:pull.
func pullScopes(scopes []string) []access {
	out := []access{}
	for _, s := range scopes {
		for _, one := range strings.Fields(s) {
			parts := strings.Split(one, ":")
			if len(parts) < 3 || parts[0] != "repository" {
				continue
			}
			// The name may contain host:port; actions are always the last field.
			name := strings.Join(parts[1:len(parts)-1], ":")
			for _, a := range strings.Split(parts[len(parts)-1], ",") {
				if a == "pull" {
					out = append(out, access{"repository", name, []string{"pull"}})
					break
				}
			}
		}
	}
	return out
}

// sign returns a compact EdDSA JWS.
func sign(key ed25519.PrivateKey, claims any) (string, error) {
	enc := base64.RawURLEncoding
	h, err := json.Marshal(map[string]string{"alg": "EdDSA", "typ": "JWT"})
	if err != nil {
		return "", err
	}
	c, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	unsigned := enc.EncodeToString(h) + "." + enc.EncodeToString(c)
	return unsigned + "." + enc.EncodeToString(ed25519.Sign(key, []byte(unsigned))), nil
}

func loadKey(path string) (ed25519.PrivateKey, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, errors.New("no PEM block")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := parsed.(ed25519.PrivateKey)
	if !ok {
		return nil, errors.New("not an Ed25519 key")
	}
	return key, nil
}

func tokenHandler(key ed25519.PrivateKey, issuer string, ttl time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// GET carries scope in the query; the OAuth2 POST form in the body.
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		now := time.Now()
		token, err := sign(key, map[string]any{
			"iss":    issuer,
			"aud":    r.Form.Get("service"),
			"iat":    now.Unix(),
			"nbf":    now.Unix(),
			"exp":    now.Add(ttl).Unix(),
			"access": pullScopes(r.Form["scope"]),
		})
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(map[string]any{
			"token":        token,
			"access_token": token,
			"expires_in":   int(ttl.Seconds()),
			"issued_at":    now.UTC().Format(time.RFC3339),
		})
	}
}

func env(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

func main() {
	key, err := loadKey(os.Getenv("KEY_FILE"))
	if err != nil {
		log.Fatalf("KEY_FILE: %v", err)
	}
	ttl, err := time.ParseDuration(env("TOKEN_TTL", "5m"))
	if err != nil {
		log.Fatalf("TOKEN_TTL: %v", err)
	}
	http.Handle("/token", tokenHandler(key, env("ISSUER", "oci-token-server"), ttl))
	http.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
	addr := env("LISTEN", ":8080")
	log.Printf("listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
