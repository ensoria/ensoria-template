package authkit_test

import (
	"context"

	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/golang-jwt/jwt/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/config/pkg/appconfig"
	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/rest/pkg/rest"
)

const (
	testSecret   = "dev-secret"
	testIssuer   = "https://issuer.example.com"
	testAudience = "ensoria"
)

// requestWith builds a request carrying the given headers (test helper).
func requestWith(headers map[string]string) *rest.Request {
	httpReq := httptest.NewRequest(http.MethodGet, "/things", nil)
	for k, v := range headers {
		httpReq.Header.Set(k, v)
	}
	return rest.NewRequest(httpReq)
}

// claims builds a valid claim set that individual tests then bend (test helper).
func claims() jwt.MapClaims {
	return jwt.MapClaims{
		"sub":   "usr_1",
		"iss":   testIssuer,
		"aud":   testAudience,
		"exp":   time.Now().Add(time.Hour).Unix(),
		"scope": "users:read users:write",
	}
}

// signHS signs a token with the shared secret (test helper).
func signHS(c jwt.MapClaims) string {
	GinkgoHelper()
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString([]byte(testSecret))
	Expect(err).NotTo(HaveOccurred())
	return token
}

// bearer wraps a token in an Authorization header value (test helper).
func bearer(token string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + token}
}

var hs256Config = &appconfig.Auth{
	Mode:     appconfig.AuthModeHS256,
	Secret:   testSecret,
	Issuer:   testIssuer,
	Audience: testAudience,
}

var _ = Describe("Verifier", func() {
	Describe("no credential at all", func() {
		It("reports that the request carried none, which is not an error yet", func() {
			verifier, err := authkit.NewVerifier(hs256Config, nil)
			Expect(err).NotTo(HaveOccurred())

			principal, err := verifier.Verify(requestWith(nil))

			Expect(principal).To(BeNil())
			Expect(err).To(MatchError(authkit.ErrNoCredential))
		})
	})

	Describe("JWT signed with a shared secret", func() {
		var verifier authkit.Verifier

		BeforeEach(func() {
			var err error
			verifier, err = authkit.NewVerifier(hs256Config, nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("accepts a valid token and reads the caller out of it", func() {
			principal, err := verifier.Verify(requestWith(bearer(signHS(claims()))))

			Expect(err).NotTo(HaveOccurred())
			Expect(principal.Subject).To(Equal("usr_1"))
			Expect(principal.Scheme).To(Equal(authkit.SchemeJWT))
			Expect(principal.Scopes).To(Equal([]string{"users:read", "users:write"}))
			Expect(principal.Claims).To(HaveKeyWithValue("iss", testIssuer))
		})

		DescribeTable("rejects a token it cannot trust",
			func(bend func(jwt.MapClaims)) {
				c := claims()
				bend(c)

				_, err := verifier.Verify(requestWith(bearer(signHS(c))))

				Expect(err).To(HaveOccurred())
				Expect(errors.Is(err, authkit.ErrNoCredential)).To(BeFalse(),
					"an unusable credential is not the same as a missing one")
			},
			Entry("expired", func(c jwt.MapClaims) { c["exp"] = time.Now().Add(-time.Hour).Unix() }),
			Entry("not yet valid", func(c jwt.MapClaims) { c["nbf"] = time.Now().Add(time.Hour).Unix() }),
			Entry("issued by someone else", func(c jwt.MapClaims) { c["iss"] = "https://evil.example.com" }),
			Entry("meant for another audience", func(c jwt.MapClaims) { c["aud"] = "other-api" }),
			Entry("no expiry at all", func(c jwt.MapClaims) { delete(c, "exp") }),
		)

		It("rejects a token signed with a different secret", func() {
			token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims()).SignedString([]byte("wrong"))
			Expect(err).NotTo(HaveOccurred())

			_, err = verifier.Verify(requestWith(bearer(token)))

			Expect(err).To(HaveOccurred())
		})

		// Accepting an asymmetrically signed token here would let anyone who knows
		// the public key mint tokens: the classic algorithm confusion attack.
		It("rejects a token signed with another algorithm", func() {
			key, err := rsa.GenerateKey(rand.Reader, 2048)
			Expect(err).NotTo(HaveOccurred())
			token, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims()).SignedString(key)
			Expect(err).NotTo(HaveOccurred())

			_, err = verifier.Verify(requestWith(bearer(token)))

			Expect(err).To(HaveOccurred())
		})

		// HS384 verifies against the very same secret, so nothing but pinning the
		// signing method stops a caller from choosing the algorithm themselves.
		It("rejects a token that picked a different HMAC algorithm", func() {
			token, err := jwt.NewWithClaims(jwt.SigningMethodHS384, claims()).SignedString([]byte(testSecret))
			Expect(err).NotTo(HaveOccurred())

			_, err = verifier.Verify(requestWith(bearer(token)))

			Expect(err).To(HaveOccurred())
		})

		It("rejects an Authorization header that is not a bearer token", func() {
			_, err := verifier.Verify(requestWith(map[string]string{"Authorization": "Basic dXNlcjpwYXNz"}))

			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, authkit.ErrNoCredential)).To(BeFalse())
		})

		It("skips the issuer and audience checks when they are not configured", func() {
			open, err := authkit.NewVerifier(&appconfig.Auth{
				Mode: appconfig.AuthModeHS256, Secret: testSecret,
			}, nil)
			Expect(err).NotTo(HaveOccurred())

			c := claims()
			c["iss"] = "https://anywhere.example.com"
			c["aud"] = "anything"

			principal, err := open.Verify(requestWith(bearer(signHS(c))))

			Expect(err).NotTo(HaveOccurred())
			Expect(principal.Subject).To(Equal("usr_1"))
		})
	})

	Describe("JWT verified against a published key set", func() {
		var verifier authkit.Verifier
		var signingKey *rsa.PrivateKey
		var server *httptest.Server

		BeforeEach(func() {
			var err error
			signingKey, err = rsa.GenerateKey(rand.Reader, 2048)
			Expect(err).NotTo(HaveOccurred())
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, jwksOf(signingKey, "key-1"))
			}))
			DeferCleanup(server.Close)

			verifier, err = authkit.NewVerifier(&appconfig.Auth{
				Mode:         appconfig.AuthModeJWKS,
				JWKSURL:      server.URL,
				Issuer:       testIssuer,
				Audience:     testAudience,
				JWKSCacheTTL: time.Hour,
			}, nil)
			Expect(err).NotTo(HaveOccurred())
		})

		signRS := func(key *rsa.PrivateKey, kid string) string {
			GinkgoHelper()
			token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims())
			token.Header["kid"] = kid
			signed, err := token.SignedString(key)
			Expect(err).NotTo(HaveOccurred())
			return signed
		}

		It("accepts a token signed by a published key", func() {
			principal, err := verifier.Verify(requestWith(bearer(signRS(signingKey, "key-1"))))

			Expect(err).NotTo(HaveOccurred())
			Expect(principal.Subject).To(Equal("usr_1"))
			Expect(principal.Scheme).To(Equal(authkit.SchemeJWT))
		})

		It("rejects a token signed by a key the issuer never published", func() {
			other, err := rsa.GenerateKey(rand.Reader, 2048)
			Expect(err).NotTo(HaveOccurred())

			_, err = verifier.Verify(requestWith(bearer(signRS(other, "key-unknown"))))

			Expect(err).To(HaveOccurred())
		})

		It("rejects a shared-secret token when the issuer signs asymmetrically", func() {
			_, err := verifier.Verify(requestWith(bearer(signHS(claims()))))

			Expect(err).To(HaveOccurred())
		})
	})

	Describe("API key", func() {
		var verifier authkit.Verifier

		BeforeEach(func() {
			var err error
			verifier, err = authkit.NewVerifier(&appconfig.Auth{
				APIKeyHeader: appconfig.DefaultAPIKeyHeader,
				APIKeys:      []string{"key-one", "key-two"},
			}, nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("accepts a configured key", func() {
			principal, err := verifier.Verify(requestWith(map[string]string{
				appconfig.DefaultAPIKeyHeader: "key-two",
			}))

			Expect(err).NotTo(HaveOccurred())
			Expect(principal.Scheme).To(Equal(authkit.SchemeAPIKey))
			Expect(principal.Subject).NotTo(BeEmpty())
		})

		It("rejects a key nobody configured", func() {
			_, err := verifier.Verify(requestWith(map[string]string{
				appconfig.DefaultAPIKeyHeader: "not-a-key",
			}))

			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, authkit.ErrNoCredential)).To(BeFalse())
		})

		// The distinction the whole error vocabulary exists for. A key store
		// that is down says nothing about the key it was asked about, and
		// answering 401 would tell every caller in the system that their
		// perfectly good key is bad at the moment nothing can check any of them.
		Describe("a key store that cannot answer", func() {
			// storeReturning builds a verifier whose key store always fails the
			// same way.
			storeReturning := func(failure error) authkit.Verifier {
				GinkgoHelper()

				v, err := authkit.NewVerifier(
					&appconfig.Auth{APIKeyHeader: appconfig.DefaultAPIKeyHeader},
					authkit.KeyStoreFunc(func(context.Context, string) (*authkit.Principal, error) {
						return nil, failure
					}),
				)
				Expect(err).NotTo(HaveOccurred())
				return v
			}

			It("is not treated as a bad credential", func() {
				v := storeReturning(errors.New("connection refused"))

				_, err := v.Verify(requestWith(map[string]string{
					appconfig.DefaultAPIKeyHeader: "a-key",
				}))

				Expect(errors.Is(err, authkit.ErrCredentialUnavailable)).To(BeTrue())
				Expect(errors.Is(err, authkit.ErrInvalidCredential)).To(BeFalse())
			})

			// The other half: a store that answered, and the answer is no.
			It("is a bad credential when the store says the key is unknown", func() {
				v := storeReturning(authkit.ErrKeyNotFound)

				_, err := v.Verify(requestWith(map[string]string{
					appconfig.DefaultAPIKeyHeader: "a-key",
				}))

				Expect(errors.Is(err, authkit.ErrInvalidCredential)).To(BeTrue())
				Expect(errors.Is(err, authkit.ErrCredentialUnavailable)).To(BeFalse())
			})
		})

		It("uses a key store the application supplies instead of the configured keys", func() {
			custom, err := authkit.NewVerifier(
				&appconfig.Auth{APIKeyHeader: appconfig.DefaultAPIKeyHeader, APIKeys: []string{"ignored"}},
				authkit.KeyStoreFunc(func(_ context.Context, key string) (*authkit.Principal, error) {
					if key != "from-store" {
						return nil, authkit.ErrKeyNotFound
					}
					return &authkit.Principal{Subject: "svc_1", Scopes: []string{"jobs:run"}}, nil
				}),
			)
			Expect(err).NotTo(HaveOccurred())

			principal, err := custom.Verify(requestWith(map[string]string{
				appconfig.DefaultAPIKeyHeader: "from-store",
			}))

			Expect(err).NotTo(HaveOccurred())
			Expect(principal.Subject).To(Equal("svc_1"))
			Expect(principal.Scopes).To(Equal([]string{"jobs:run"}))
		})
	})

	Describe("building the verifier", func() {
		It("refuses a shared-secret setup with no secret", func() {
			_, err := authkit.NewVerifier(&appconfig.Auth{Mode: appconfig.AuthModeHS256}, nil)

			Expect(err).To(HaveOccurred())
		})

		It("refuses a key-set setup with no URL", func() {
			_, err := authkit.NewVerifier(&appconfig.Auth{Mode: appconfig.AuthModeJWKS}, nil)

			Expect(err).To(HaveOccurred())
		})

		It("refuses a mode it does not know", func() {
			_, err := authkit.NewVerifier(&appconfig.Auth{Mode: "magic", Secret: "s"}, nil)

			Expect(err).To(HaveOccurred())
		})

		It("builds an API-key-only verifier when no JWT mode is configured", func() {
			verifier, err := authkit.NewVerifier(&appconfig.Auth{
				APIKeyHeader: appconfig.DefaultAPIKeyHeader, APIKeys: []string{"k"},
			}, nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(verifier).NotTo(BeNil())
		})
	})
})

// jwksOf publishes one RSA public key as a JWK Set document (test helper).
func jwksOf(key *rsa.PrivateKey, kid string) string {
	n := base64.RawURLEncoding.EncodeToString(key.N.Bytes())
	e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())
	return fmt.Sprintf(`{"keys":[{"kty":"RSA","kid":%q,"use":"sig","alg":"RS256","n":%q,"e":%q}]}`, kid, n, e)
}
