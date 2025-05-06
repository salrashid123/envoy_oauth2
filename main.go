package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"time"

	"net/http/httputil"

	"flag"

	"github.com/gorilla/mux"
	"golang.org/x/net/http2"
	"golang.org/x/oauth2"
	oauth2svc "google.golang.org/api/oauth2/v2"
	"google.golang.org/api/option"
)

var (
	validateUser = flag.Bool("validateUser", false, "Lookup User with oauth2 tokeninfo endpoint")
)

const (
	hmacKey = "93Wg15rHSp6/Si5bH756OE6mAqL9ntX5DQ7ug5NgncE="
)

type contextKey string

const contextEventKey contextKey = "event"

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

		hc, err := r.Cookie("OauthHMAC")
		if err != nil {
			http.Error(w, fmt.Sprint(err), http.StatusUnauthorized)
			return
		}

		expires, err := r.Cookie("OauthExpires")
		if err != nil {
			http.Error(w, fmt.Sprint(err), http.StatusUnauthorized)
			return
		}

		host := r.Host
		if host == "" {
			http.Error(w, fmt.Sprint(err), http.StatusUnauthorized)
			return
		}

		accessToken := ""
		idToken := ""
		message := ""

		bearerTokenCookie, err := r.Cookie("BearerToken")
		if err != nil {
			http.Error(w, "BearerToken not present", http.StatusUnauthorized)
			return
		}
		accessToken = bearerTokenCookie.Value

		idTokenCookie, err := r.Cookie("IdToken")
		if err != nil {
			http.Error(w, "IdToken not present", http.StatusUnauthorized)
			return
		}
		idToken = idTokenCookie.Value

		message = fmt.Sprintf("%s\n%s\n%s\n%s\n", host, expires.Value, accessToken, idToken)

		// https://github.com/envoyproxy/envoy/blob/main/source/extensions/filters/http/oauth2/filter.cc#L177C22-L177C69

		hsh := hmac.New(sha256.New, []byte(hmacKey))
		hsh.Write(([]byte(message)))

		calculatedHMAC := base64.StdEncoding.EncodeToString(hsh.Sum(nil))

		if hc.Value != calculatedHMAC {
			http.Error(w, "HMAC validation Failed", http.StatusUnauthorized)
			return
		}

		ctx := context.Background()

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
