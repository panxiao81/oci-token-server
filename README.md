# oci-token-server

A tiny token server for OCI registries that implements the
[distribution token authentication spec](https://distribution.github.io/distribution/spec/auth/token/).

Right now it does one thing: **anonymous pull**. Any request gets a short-lived
token whose `repository:<name>:pull` scopes are kept and everything else
(push, delete, catalog, …) is dropped. Credentials in the request are ignored.

This is the same model Docker Hub uses for anonymous pulls: the client hits
`/v2/`, receives a `401` with a Bearer challenge, asks the token server for a
token without credentials, and gets a pull-only one. docker, containerd and
podman all follow this flow.

## Why

Some registries can't serve anonymous pulls and authenticated pushes from the
same endpoint. zot (as of v2.1.21) with `bearer.oidc` rejects requests without a
token before authorization runs, so the usual workaround is a second, read-only
instance on a second hostname. With this server behind zot's
`upstreamTokenEndpoint`, a single instance does both:

- credentials zot recognises (e.g. an OIDC / SPIFFE JWT-SVID used as the
  `docker login` password) are verified by zot itself;
- requests without credentials are proxied here and come back with a
  pull-only token, which zot verifies with `bearer.cert`.

## Configuration

| Variable    | Default            | Description |
|-------------|--------------------|-------------|
| `KEY_FILE`  | (required)         | Ed25519 private key, PKCS#8 PEM. |
| `ISSUER`    | `oci-token-server` | `iss` claim. |
| `TOKEN_TTL` | `5m`               | Token lifetime (Go duration). |
| `LISTEN`    | `:8080`            | Listen address. Serves `/token` and `/healthz`. |

The `aud` claim is the `service` parameter of the request.

```sh
openssl genpkey -algorithm ed25519 -out key.pem
openssl pkey -in key.pem -pubout -out pub.pem   # give this to the registry
docker run -v $PWD/key.pem:/key.pem:ro -e KEY_FILE=/key.pem -p 8080:8080 \
  ghcr.io/panxiao81/oci-token-server:main
```

## zot

```json
"auth": {
  "bearer": {
    "realm": "https://registry.example.com/zot/auth/token",
    "service": "registry.example.com",
    "cert": "/etc/zot/pub.pem",
    "upstreamTokenEndpoint": {
      "realm": "http://oci-token-server:8080/token",
      "service": "registry.example.com",
      "allowInsecureHttp": true
    },
    "oidc": [{ "issuer": "https://issuer.example.com", "audiences": ["zot"] }]
  }
}
```

Tested with zot v2.1.21: anonymous pull via docker, containerd (`ctr`) and
podman; anonymous push is rejected; pushes with a JWT-SVID keep working.

`allowInsecureHttp` is only reasonable when the hop stays inside the cluster.
zot forwards credentials it does not recognise (e.g. a mistyped password) to the
upstream as-is, so use HTTPS if that hop leaves a trusted network.

Other registries (distribution, …) have not been tested. distribution checks
`iss` against its configured issuer and needs to find the verification key, so
it likely needs at least a matching `ISSUER` and a JWKS configuration.

## Roadmap

Accept JWTs (OIDC ID tokens, SPIFFE JWT-SVIDs, CI OIDC tokens) as the
`docker login` password, verify them against the issuer's JWKS and grant push
scopes from a policy file, so the registry only has to verify tokens.

## License

MIT
