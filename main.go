package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"net/http/httputil"

	"flag"

	"cloud.google.com/go/auth/credentials/idtoken"
	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/mux"
	"github.com/lestrrat/go-jwx/jwk"
	"golang.org/x/net/http2"
	"golang.org/x/oauth2"
	oauth2svc "google.golang.org/api/oauth2/v2"
	"google.golang.org/api/option"
)

var (
	validateUser = flag.Bool("validateUser", false, "Lookup User with oauth2 tokeninfo endpoint")
)

var (
	jwtSet *jwk.Set
)

const (
	hmacKey = "93Wg15rHSp6/Si5bH756OE6mAqL9ntX5DQ7ug5NgncE="
)

type contextKey string

const (
	audience                   = "839905111702-vrh3m9blh9d7m25dj2cs8fv2v5qle3oa.apps.googleusercontent.com"
	contextEventKey contextKey = "event"
)

type parsedData struct {
	AccessToken string `json:"access_token"`
	Subject     string `json:"sub,omitempty"`
}

func oauthMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		dump, err := httputil.DumpRequest(r, true)
		if err != nil {
			http.Error(w, fmt.Sprint(err), http.StatusInternalServerError)
			return
		}

		fmt.Printf("Headers: %s\n", dump)

		ctx := context.Background()

		// extract and validate id_token
		idToken := ""
		idTokenCookie, err := r.Cookie("IdToken")
		if err != nil {
			http.Error(w, "IdToken not present", http.StatusUnauthorized)
			return
		}
		idToken = idTokenCookie.Value

		//fmt.Printf("idToken: %s\n", idToken)

		validTok, err := idtoken.Validate(ctx, idToken, audience)
		if err != nil {
			log.Fatalf("token validation failed: %v", err)
		}
		if validTok.Audience != audience {
			log.Fatalf("got %q, want %q", validTok.Audience, audience)
		}

		// If you want to verify any other issuer, first get the JWK endpoint,
		// in the following we are validating google's tokens, meaning its equivalent to the bit above
		// this is added in as an example of verifying IAP or other token types
		jwksURL := "https://www.googleapis.com/oauth2/v3/certs"
		jwtSet, err = jwk.FetchHTTP(jwksURL)
		if err != nil {
			log.Fatal("Unable to load JWK Set: ", err)
		}
		doc, err := verifyGoogleIDToken(ctx, idToken)
		if err != nil {
			log.Fatalf("Unable to verify IDTOKEN: %v", err)
		}
		log.Printf("Verified Token with subject: %v", doc.Email)

		// use access token against an API, in this case, it happens to be the oauth api

		accessToken := ""
		bearerTokenCookie, err := r.Cookie("BearerToken")
		if err != nil {
			http.Error(w, "BearerToken not present", http.StatusUnauthorized)
			return
		}
		accessToken = bearerTokenCookie.Value

		//	fmt.Printf("accessToken: %s\n", accessToken)

		rootTS := oauth2.StaticTokenSource(&oauth2.Token{
			AccessToken: accessToken,
			TokenType:   "Bearer",
			Expiry:      time.Now().Add(time.Duration(time.Second * 60)),
		})

		oauth2Service, err := oauth2svc.NewService(ctx, option.WithTokenSource(rootTS))
		if err != nil {
			fmt.Printf("%v\n", err)
			http.Error(w, fmt.Sprint(err), http.StatusInternalServerError)
			return
		}
		tokenInfoCall := oauth2Service.Tokeninfo()
		tokenInfoCall.AccessToken(accessToken)
		tokenInfo, err := tokenInfoCall.Do()
		if err != nil {
			fmt.Printf("%v\n", err)
			http.Error(w, fmt.Sprint(err), http.StatusInternalServerError)
			return
		}
		userid := tokenInfo.Email
		event := &parsedData{
			AccessToken: accessToken,
			Subject:     userid,
		}

		rctx := context.WithValue(r.Context(), contextEventKey, *event)
		h.ServeHTTP(w, r.WithContext(rctx))
	})
}

func gethandler(w http.ResponseWriter, r *http.Request) {

	val := r.Context().Value(contextKey("event")).(parsedData)
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "User %v logged in.", val.Subject)

}

func main() {

	flag.Parse()
	router := mux.NewRouter()
	router.Methods(http.MethodGet).Path("/get").HandlerFunc(gethandler)

	server := &http.Server{
		Addr:    ":8082",
		Handler: oauthMiddleware(router),
	}
	http2.ConfigureServer(server, &http2.Server{})
	fmt.Println("Starting Server..")
	err := server.ListenAndServeTLS("certs/backend.crt", "certs/backend.key")
	fmt.Printf("Unable to start Server %v", err)

}

type gcpIdentityDoc struct {
	Google struct {
		ComputeEngine struct {
			InstanceCreationTimestamp int64  `json:"instance_creation_timestamp,omitempty"`
			InstanceID                string `json:"instance_id,omitempty"`
			InstanceName              string `json:"instance_name,omitempty"`
			ProjectID                 string `json:"project_id,omitempty"`
			ProjectNumber             int64  `json:"project_number,omitempty"`
			Zone                      string `json:"zone,omitempty"`
		} `json:"compute_engine"`
	} `json:"google"`
	Email           string `json:"email,omitempty"`
	EmailVerified   bool   `json:"email_verified,omitempty"`
	AuthorizedParty string `json:"azp,omitempty"`
	jwt.RegisteredClaims
}

func getKey(token *jwt.Token) (interface{}, error) {
	keyID, ok := token.Header["kid"].(string)
	if !ok {
		return nil, errors.New("expecting JWT header to have string kid")
	}
	if key := jwtSet.LookupKeyID(keyID); len(key) == 1 {
		log.Printf("     Found OIDC KeyID  " + keyID)
		return key[0].Materialize()
	}
	return nil, errors.New("unable to find key")
}

func verifyGoogleIDToken(ctx context.Context, rawToken string) (gcpIdentityDoc, error) {
	token, err := jwt.ParseWithClaims(rawToken, &gcpIdentityDoc{}, getKey)
	if err != nil {
		log.Printf("     Error parsing JWT %v", err)
		return gcpIdentityDoc{}, err
	}
	if claims, ok := token.Claims.(*gcpIdentityDoc); ok && token.Valid {
		log.Printf("     OIDC doc has Audience [%s]   Issuer [%s] and SubjectEmail [%s]", claims.Audience, claims.RegisteredClaims.Issuer, claims.Email)
		return *claims, nil
	}
	return gcpIdentityDoc{}, errors.New("Error parsing JWT Claims")
}
