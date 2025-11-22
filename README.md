## Envoy Oauth2 Filter

A simple sample demonstrating [Envoy's Oauth2 Filter](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/oauth2_filter).

Basically, this filter will handle all the details for [OAuth 2.0 for Web Server Applications](https://developers.google.com/identity/protocols/oauth2/web-server) and once a user is validated, it will forward the user to the backend application.

Web applications can certainly handle the oauth2 flow (see [flask plugin](https://flask-oauthlib.readthedocs.io/en/latest/oauth2.html)) but this filter manages the sessions for you and after a successful login, provides an `HMAC` confirmation that a login happened and optionally the raw `access_token` for the user that logged in.

As with the nature of envoy, this configuration will act to do all the legwork for you and present a backend service you run with the user's oauth2 authentication token (i.,e envoy does the whole oauth2 flow for you).

At a high level, its basically

1. user access a url handled by envoy
2. envoy presents user with oauth2 flow and redirects to google
3. user logs into google and is redirected back to envoy
4. envoy completes the oauth2 flow and acquires the user's `access_token`.
5. envoy signs an hmac cookie and sends that to the user along with a redirect to the url requested in `1`
6. user requests the URL and provides the hmac cookies forward
7. envoy verifies the cookies and forwards the requests to the backend server
8. backend server verifies the hmac values match and extracts optionally the `access_token`

![images/login_flow.png](images/login_flow.png)

Note, part of this tutorial is inspired by [veehaitch@](https://github.com/veehaitch/envoy-oauth2-filter-google).  The enhancement i added is to do hmac validation.

### Setup

This tutorial runs envoy and backend server locally for testing. Envoy will run on port `:8081` while the backend server on `:8082`, both over TLS.


#### Configure client_id/secret

The first step is to configure an oauth2 `client_id` and `client_secret`.  For google cloud, configure one [here](https://developers.google.com/identity/gsi/web/guides/get-google-api-clientid).

For this tutorial, you can set the `Authorized Redirect Uri` value to `https://envoy.esodemoapp2.com:8081/callback`.  
![images/client_id.png](images/client_id.png)


Note, I've setup DNS resolution on that domain to point back to "localhost" (which is where this tutorial takes place and where envoy and backend servers run)

```
$ nslookup envoy.esodemoapp2.com 8.8.8.8
Name:	envoy.esodemoapp2.com
Address: 127.0.0.1

$ nslookup backend.esodemoapp2.com 8.8.8.8
Name:	backend.esodemoapp2.com
Address: 127.0.0.1
```

Once you have the `client_id` and `secret`, 

for the `client_id`, edit `proxy.yaml` and set the value:

```yaml
    credentials:
      client_id: "248066739582-h498t6035hm9lvp5u9jelm8i67rp43vq.apps.googleusercontent.com"
```

for the `client_secret`, edit `token-secret.yaml` file and enter it in there

also note, the HMAC secret is also specified in a file appropriately named `hmac-secret.yaml`


The `token-secret`, `client_id` and `client_secret` are now all set


#### Start Envoy

First get the latest envoy binary:

```
 docker cp `docker create envoyproxy/envoy-dev:latest`:/usr/local/bin/envoy .
```

Then just run envoy

```
./envoy --base-id 0 -c proxy.yaml -l trace
```

#### Start backend service

Now run the backend service webserver

```
go run main.go
```

In an incognito browser, goto 

* [https://envoy.esodemoapp2.com:8081/get](https://envoy.esodemoapp2.com:8081/get)

This will redirect you back to google oauth2 login screens where you can login.

Once logged in, you'll get redirected back though envoy and ultimately to the backend service.

THe backend service will receive the following

* `Host`: the standard host header
* `BearerToken`:  this is the raw oauth2 `access_token`.  This value is optionally enabled using the `forward_bearer_token: true` flag in `proxy.yaml`
* `Cookie`: which includes the `id_token` for the user.  The application can verify the validity of this id_token if it wants to but its already validated by envoy.

The provided backend service does one optional flow as well:  it uses [oauth2 tokeninfo](https://pkg.go.dev/google.golang.org/api@v0.63.0/oauth2/v2) endpoint to determine who the user is

You can also terminate envoy's session by invoking the `/signout` url at anytime.  This will invalidate all the cookies.

One more thing to note, while users can use any system to perform oauth2 flows, [Scopes](https://developers.google.com/identity/protocols/oauth2/scopes) are [restricted or sensitive](https://support.google.com/cloud/answer/9110914).  In other words, you can't just ask a user for their `cloud-platform` enabled `access_token` and start doing stuff.

---

#### Traffic flow

1. User visits `https://envoy.esodemoapp2.com:8081/get`

   If the user is not logged in, redirect to google login


2. Login to google

   Redirect post login to `https://envoy.esodemoapp2.com:8081/callback`


```bash
GET /callback?state=eyJ1cmwiOiJodHRwczovL2Vudm95LmVzb2RlbW9hcHAyLmNvbTo4MDgxL2dldCIsImNzcmZfdG9rZW4iOiIwOWYwYzY1N2JjNjIxMjhjLkxpcjRJZUlFQUFrd0NMczZTTTFpYS9seEJ6Ry9pekJxMkFKRlNmZStCLzg9In0&code=4%2F0Ab32j93gudVEHFqF15hzhZx2tcbwquFY_I-3c4REbgV-Lredacted&scope=email+https%3A%2F%2Fwww.googleapis.com%2Fauth%2Fuserinfo.email+openid&authuser=0&prompt=none HTTP/1.1
Cookie: OauthNonce=09f0c657bc62128c.Lir4IeIEAAkwCLs6SM1ia/lxBzG/izBq2AJFSfe+B/8=; CodeVerifier=ymKfhhm_UnEgMsSbIdtdOhHa1pCfGbZjBlSRSizD9zmdfYmun9no-Em3UN8Yj9Hi_yDyN_Lu0-QlSaIwhRanjw
Host: envoy.esodemoapp2.com:8081
Referer: https://accounts.google.co.jp/


HTTP/1.1 302 Found
set-cookie: OauthHMAC=FUotR4F5mLDgnpN3EphG6LPHxDJGYU4kQU4GpmoEnrI=;path=/;Max-Age=3599;secure;HttpOnly
set-cookie: OauthExpires=1763696193;path=/;Max-Age=3599;secure;HttpOnly
set-cookie: BearerToken=VoHdwuNJ4G0LdVirniUcLqq1B297dowkyzPX4rI1KDjZbOww-EoUk8p2VJP4E8I1loReleSs108p4NH002KC9H18BHtS0kRv74DocdXJUebqPdZ86CXs0DM1KTO4TzJ8wL3hGx16YInWyOJI9ObO3yH-8k0GN3XoxckY_TznJZllEh0-j0yL6gYkWp4NuO6n5NSsDjbiRscWmjd2A9PolZos9vuYdCOJ4497tquclZN_UogL_mZEhojIy0aTq6fAN_dlMsmfcA_pnqsKHHecbTNuB4U_9rUCKtnxbpnFvioVQdHTwqJ5btjG47qVaNyGmJ48Hqac-oKLLOwLupTcfSvEUN-EVO9eDAdPCI9ifOBOZ2IcfzIED3Rlxvdu7eeRZwXOLzGG73d6-dDrCM0K9y3LuXb4gLX_EYFrWSGs3lrj5s1nB7Wz13gfUZlQpdaJZXMwFOqLK_Nbredacted;path=/;Max-Age=3599;secure;HttpOnly
set-cookie: IdToken=qfD3oXZNGuvlm7rtpnJc3lWenha0HGdwTwvfOvItnzaxEjcA7Da-qViA-47GGCEeYnY1xpyF7rFYOxAk232a5TUHIbDD66tnbggzBhKHUGCV2Zqt3Kv9IeRZfpY3JJOH4nvW-fzzdcfnOVQxQ75NoB1JQSnd8aBJR0OZeIKIBSiZ_7nkoCv-oL4M0TXVXcnZF7uMB8KSaMv7tN4FdofVDQxWho6bDX5ZC7G5DlSHsIyxfyzrJN3_ZlCQKcKXor8ucacoQPbJKqu9zi8OgnEbNOQ7X253cCcME984PGE3AIVKSC8Mybyoge51dcj-JJEAMGmV5cvljniP_rgg7B4VIgQSoIhJikB42byJNa3C0CrtBhhUXJbR7SDR2l4VPld5fp_MkjA59FBOx6ZeEF5HOwIzKnfY9dARLEpEv6GdkPyu80fLUvrHVKh9c961rXTBMlYxriXddNFM98oPAiQHhzyWuM-jhsfxWJPoi0nNlJXyn3q_Jgh2ztNGYLixLkQwu9cGZx8gBhzHnILZ3XzNfzlq_h2dKTHBWMduzgVYQlw1MdR6YEvP-w5Ohm5lWrR3JhZB8PwMXqkk1wLEvE6AoxWD88zMfuoolQXe0uY9eyIewUUpQADQlvZa4JjgLGYGWFUFxQdfwZU_KJ1cNLUk1ClK39-I24QE5zHis2qotMb3VkqUtDicHogE_js3UT1YfVbfRITUXpxxYimnn_GUA6_6DRLGOInUYP43Zv9OYxon3DbkZJ1mS13f5dDRi8yzeOGH4DppTDA1Yp7aJT4idFvOBmhfVMUIf3mfug61piZX6lQLsRA5OXntAyEwhwCzkG57l6n8ARGcO-qVSJKEJXkKYd21kPU6MeJgacE0k8ZWhs_Q_RDhq50OU_yGL9PkWyHpAr2-knhB29ntwEDF5eYw3cI4VOIPxFdA1y5qYeCiugMIJVhdqXq9pu1Gr8L088zwsMfzJedAwEeLLIq3y58Mq-6Q6Q4SB8sWGrgog2Mg_H8q3Uo7iSEY6D70n0SRMlAe8zrjfEDLiMNs5IE5qzlNwhw47PrxR76aZauj6sSFgsDtpKV2nGDKOak5WSKdPh9kjmGEueIIHX64su_6hVy9vQykVT_uw_jW0qG2JUboo-NY_ZvIzGXiN62Tne3AayMj4tw3_tdTzP00Rnm-G-Hb1iApgxWy8JrYO5OHPRyHTEslLc8VOsR92jV_j-4redacted;path=/;Max-Age=3602;secure;HttpOnly
location: https://envoy.esodemoapp2.com:8081/get
date: Fri, 21 Nov 2025 02:36:34 GMT
server: envoy
content-length: 0
```


THe response sets the envoy encrypted cookie data `BearerToken`, `IdToken` and integrity proteciton HMAC.  The encryption key uses the hmac secret value envoy has.


2. Redirect to `https://envoy.esodemoapp2.com:8081/get`

```bash
GET /get HTTP/1.1

Cookie: OauthNonce=09f0c657bc62128c.Lir4IeIEAAkwCLs6SM1ia/lxBzG/izBq2AJFSfe+B/8=; CodeVerifier=ymKfhhm_UnEgMsSbIdtdOhHa1pCfGbZjBlSRSizD9zmdfYmun9no-Em3UN8Yj9Hi_yDyN_Lu0-QlSaIwhRanjw; OauthHMAC=FUotR4F5mLDgnpN3EphG6LPHxDJGYU4kQU4GpmoEnrI=; OauthExpires=1763696193; BearerToken=VoHdwuNJ4G0LdVirniUcLqq1B297dowkyzPX4rI1KDjZbOww-EoUk8p2VJP4E8I1loReleSs108p4NH002KC9H18BHtS0kRv74DocdXJUebqPdZ86CXs0DM1KTO4TzJ8wL3hGx16YInWyOJI9ObO3yH-8k0GN3XoxckY_TznJZllEh0-j0yL6gYkWp4NuO6n5NSsDjbiRscWmjd2A9PolZos9vuYdCOJ4497tquclZN_UogL_mZEhojIy0aTq6fAN_dlMsmfcA_pnqsKHHecbTNuB4U_9rUCKtnxbpnFvioVQdHTwqJ5btjG47qVaNyGmJ48Hqac-oKLLOwLupTcfSvEUN-EVO9eDAdPCI9ifOBOZ2IcfzIED3Rlxvdu7eeRZwXOLzGG73d6-dDrCM0K9y3LuXb4gLX_EYFrWSGs3lrj5s1nB7Wz13gfUZlQpdaJZXMwFOqLK_redacted; IdToken=qfD3oXZNGuvlm7rtpnJc3lWenha0HGdwTwvfOvItnzaxEjcA7Da-qViA-47GGCEeYnY1xpyF7rFYOxAk232a5TUHIbDD66tnbggzBhKHUGCV2Zqt3Kv9IeRZfpY3JJOH4nvW-fzzdcfnOVQxQ75NoB1JQSnd8aBJR0OZeIKIBSiZ_7nkoCv-oL4M0TXVXcnZF7uMB8KSaMv7tN4FdofVDQxWho6bDX5ZC7G5DlSHsIyxfyzrJN3_ZlCQKcKXor8ucacoQPbJKqu9zi8OgnEbNOQ7X253cCcME984PGE3AIVKSC8Mybyoge51dcj-JJEAMGmV5cvljniP_rgg7B4VIgQSoIhJikB42byJNa3C0CrtBhhUXJbR7SDR2l4VPld5fp_MkjA59FBOx6ZeEF5HOwIzKnfY9dARLEpEv6GdkPyu80fLUvrHVKh9c961rXTBMlYxriXddNFM98oPAiQHhzyWuM-jhsfxWJPoi0nNlJXyn3q_Jgh2ztNGYLixLkQwu9cGZx8gBhzHnILZ3XzNfzlq_h2dKTHBWMduzgVYQlw1MdR6YEvP-w5Ohm5lWrR3JhZB8PwMXqkk1wLEvE6AoxWD88zMfuoolQXe0uY9eyIewUUpQADQlvZa4JjgLGYGWFUFxQdfwZU_KJ1cNLUk1ClK39-I24QE5zHis2qotMb3VkqUtDicHogE_js3UT1YfVbfRITUXpxxYimnn_GUA6_6DRLGOInUYP43Zv9OYxon3DbkZJ1mS13f5dDRi8yzeOGH4DppTDA1Yp7aJT4idFvOBmhfVMUIf3mfug61piZX6lQLsRA5OXntAyEwhwCzkG57l6n8ARGcO-qVSJKEJXkKYd21kPU6MeJgacE0k8ZWhs_Q_RDhq50OU_yGL9PkWyHpAr2-knhB29ntwEDF5eYw3cI4VOIPxFdA1y5qYeCiugMIJVhdqXq9pu1Gr8L088zwsMfzJedAwEeLLIq3y58Mq-6Q6Q4SB8sWGrgog2Mg_H8q3Uo7iSEY6D70n0SRMlAe8zrjfEDLiMNs5IE5qzlNwhw47PrxR76aZauj6sSFgsDtpKV2nGDKOak5WSKdPh9kjmGEueIIHX64su_6hVy9vQykVT_uw_jW0qG2JUboo-NY_ZvIzGXiN62Tne3AayMj4tw3_tdTzP00Rnm-G-Hb1iApgxWy8JrYO5OHPRyHTEslLc8VOsR92jV_j-4nxbqXLyMMJ9IDt2Tn-2xuPcNaCT25lMq8W-redacted
Host: envoy.esodemoapp2.com:8081
Referer: https://accounts.google.co.jp/

HTTP/1.1 200 OK
content-type: text/plain
content-length: 38
date: Fri, 21 Nov 2025 02:36:35 GMT
x-envoy-upstream-service-time: 163
server: envoy
```

Note, the `BearerToken` and `IdToken=` cookies from user->envoy are encrypted by envoy using the hmackey it ihas

These then get validated and decrypted and forwarded to the backend:

When you see the `/get` request on envoy, a connection to the go app is made passwing the `bearer` and `cookie`.


```bash
$ go run main.go 
Starting Server..

Headers: GET /get HTTP/2.0
Host: envoy.esodemoapp2.com:8081

Authorization: Bearer ya29.A0ATi6K2tc9NwdHWlZFkAQPDTrIdTpOaMK0d-wZIibUtZNwDYwDnbHqYaHal_noCOGWf6H017kwfwsqRF2kzhxUEdTucr2oxNOhsjIPh0UjJ7EOtsxmVEEXRfvtyqGOCrawbRur0mQ-f4KWn-wo8Gjei29exo0tVq0pbbKH5FTB9FbpCaqMtUc-z0ksnSYDsYnXB--redacted

Cookie: 
    BearerToken=ya29.A0ATi6K2tc9NwdHWlZFkAQPDTrIdTpOaMK0d-wZIibUtZNwDYwDnbHqYaHal_noCOGWf6H017kwfwsqRF2kzhxUEdTucr2oxNOhsjIPh0UjJ7EOtsxmVEEXRfvtyqGOCrawbRur0mQ-f4KWn-wo8Gjei29exo0tVq0pbbKH5FTB9FbpCaqMtUc-redacted
    
    IdToken=eyJhbGciOiJSUzI1NiIsImtpZCI6ImE1NzMzYmJiZDgxOGFhNWRiMTk1MTk5Y2Q1NjhlNWQ2ODUxMzJkM2YiLCJ0eXAiOiJKV1QifQ.eyJpc3MiOiJodHRwczovL2FjY291bnRzLmdvb2dsZS5jb20iLCJhenAiOiI4Mzk5MDUxMTE3MDItdHVyc2tvYW0wdmhta21oamVmNG5ndTdlNGZ0aDY3ZG4uYXBwcy5nb29nbGV1c2VyY29udGVudC5jb20iLCJhdWQiOiI4Mzk5MDUxMTE3MDItdHVyc2tvYW0wdmhta21oamVmNG5ndTdlNGZ0aDY3ZG4uYXBwcy5nb29nbGV1c2VyY29udGVudC5jb20iLCJzdWIiOiIxMDY5OTgwMjQ1NDYwOTIxNzkxOTIiLCJlbWFpbCI6InNhbHJhc2hpZDEyM0BnbWFpbC5jb20iLCJlbWFpbF92ZXJpZmllZCI6dHJ1ZSwiYXRfaGFzaCI6Ii1heG1MWW1ZSVFCOXQtQU1jRExURlEiLCJpYXQiOjE3NjM2OTI1OTYsImV4cCI6MTc2MzY5N-redacted

Referer: https://accounts.google.co.jp/
```