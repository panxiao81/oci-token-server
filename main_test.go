package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestPullScopes(t *testing.T) {
	got := pullScopes([]string{
		"repository:team/app:pull,push",
		"repository:a:push repository:localhost:5000/b:pull",
		"registry:catalog:*",
	})
	want := []access{
		{"repository", "team/app", []string{"pull"}},
		{"repository", "localhost:5000/b", []string{"pull"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
}

func TestTokenHandler(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	srv := httptest.NewServer(tokenHandler(priv, "test", time.Minute))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "?service=reg&scope=repository:x/y:pull,push")
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}

	parts := strings.Split(body.Token, ".")
	sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
	if !ed25519.Verify(pub, []byte(parts[0]+"."+parts[1]), sig) {
		t.Fatal("bad signature")
	}
	raw, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var claims struct {
		Iss    string   `json:"iss"`
		Aud    string   `json:"aud"`
		Access []access `json:"access"`
	}
	if err := json.Unmarshal(raw, &claims); err != nil {
		t.Fatal(err)
	}
	want := []access{{"repository", "x/y", []string{"pull"}}}
	if claims.Iss != "test" || claims.Aud != "reg" || !reflect.DeepEqual(claims.Access, want) {
		t.Fatalf("claims %+v", claims)
	}
}
