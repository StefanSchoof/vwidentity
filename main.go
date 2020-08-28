package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/publicsuffix"
	"gopkg.in/yaml.v2"
)

// with help from https://github.com/thomasesmith/vw-car-net-api

// auth urls
const loginURL = "https://www.volkswagen.de/app/authproxy/login?fag=vw-de&scope-vw-de=profile,address,phone,carConfigurations,dealers,cars,vin,profession&prompt-vw-de=login&redirectUrl=https://www.volkswagen.de/de/besitzer-und-nutzer/myvolkswagen.html"
const identifierURL = "https://identity.vwgroup.io/signin-service/v1/4fb52a96-2ba3-4f99-a3fc-583bb197684b@apps_vw-dilab_com/login/identifier"
const authenticateURL = "https://identity.vwgroup.io/signin-service/v1/4fb52a96-2ba3-4f99-a3fc-583bb197684b@apps_vw-dilab_com/login/authenticate"
const tokenURL = "https://www.volkswagen.de/app/authproxy/vw-de/tokens"
const userInfoURL = "https://www.volkswagen.de/app/authproxy/vw-de/user"

// api urls
const realCarURLFormat = "https://customer-profile.apps.emea.vwapps.io/v2/customers/%s/realCarData"

type conf struct {
	Mail     string `yaml:"mail"`
	Password string `yaml:"password"`
}

func (c *conf) getConf() *conf {
	yamlFile, err := ioutil.ReadFile("vw.yaml")
	if err != nil {
		log.Printf("yamlFile.Get err   #%v ", err)
	}
	err = yaml.Unmarshal(yamlFile, c)
	if err != nil {
		log.Fatalf("Unmarshal: %v", err)
	}

	return c
}

func main() {
	var c conf
	c.getConf()
	info, err := getAuthInfo(c.Mail, c.Password)
	if err != nil {
		log.Fatal(err)
	}
	var bearer = "Bearer " + info.BearerAccessToken

	realCarURL := fmt.Sprintf(realCarURLFormat, info.Sub)
	req, err := http.NewRequest("GET", realCarURL, nil)
	req.Header.Add("Authorization", bearer)
	client := &http.Client{}
	resp, err := executeRequest(req, *client)
	if err != nil {
		log.Fatal(err)
	}

	body, _ := ioutil.ReadAll(resp.Body)
	log.Println(string([]byte(body)))
}

type authProxyTokens struct {
	AccessToken string `json:"access_token"`
	IDToken     string `json:"id_token"`
}

type authInfo struct {
	BearerAccessToken string
	Sub               string
	Name              string
	GivenName         string `json:"given_name"`
	FamilyName        string `json:"family_name"`
	Email             string
	EmailVerified     bool  `json:"email_verified"`
	UpdatedAt         int64 `json:"updated_at"`
}

func getAuthInfo(mail string, password string) (info authInfo, err error) {
	client, err := getHTTPClient()
	if err != nil {
		return info, err
	}

	err = logIn(*client, mail, password)
	if err != nil {
		return info, err
	}

	accessToken, err := getAccessToken(*client)
	if err != nil {
		return info, err
	}
	info, err = getUserInfo(*client)
	if err != nil {
		return info, err
	}
	info.BearerAccessToken = accessToken
	return info, nil
}

func getHTTPClient() (client *http.Client, err error) {
	jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	if err != nil {
		return client, err
	}

	return &http.Client{
		Jar: jar,
	}, nil
}

func logIn(client http.Client, mail string, password string) error {
	idValues, err := getStartValues(client)
	if err != nil {
		return err
	}

	form := url.Values{
		"_csrf":      []string{idValues["csrf"]},
		"relayState": []string{idValues["input_relayState"]},
		"hmac":       []string{idValues["hmac"]},
		"email":      []string{mail},
	}
	hmac, err := getHmacFromMail(client, form)
	if err != nil {
		return err
	}
	form.Set("hmac", hmac)
	form.Set("password", password)
	return authenticate(client, form)
}

func getStartValues(client http.Client) (map[string]string, error) {
	req, err := http.NewRequest("GET", loginURL, nil)
	res, err := executeRequest(req, client)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	return getInputValues(res.Body, []string{"csrf", "input_relayState", "hmac"})
}

func getUserInfo(client http.Client) (info authInfo, err error) {
	csrf, err := getCsrfToken(client.Jar)
	if err != nil {
		return info, err
	}
	req, err := http.NewRequest("GET", userInfoURL, nil)
	req.Header.Add("x-csrf-token", csrf)
	res, err := executeRequest(req, client)
	if err != nil {
		log.Fatal(err)
	}
	decoder := json.NewDecoder(res.Body)
	err = decoder.Decode(&info)
	return info, nil
}

func getHmacFromMail(client http.Client, formValues url.Values) (string, error) {
	res, err := executePostForm(identifierURL, formValues, client)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	idValues, err := getInputValues(res.Body, []string{"hmac"})
	if err != nil {
		return "", err
	}
	return idValues["hmac"], nil
}

func authenticate(client http.Client, formValues url.Values) error {
	res, err := executePostForm(authenticateURL, formValues, client)
	if err != nil {
		return err
	}
	if strings.HasSuffix(res.Request.URL.Path, "terms-and-conditions") {
		return fmt.Errorf("the terms and conditions have updated. Please log in with the browser and accecept them")
	}
	return nil
}

func getAccessToken(client http.Client) (string, error) {
	csrf, err := getCsrfToken(client.Jar)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest("GET", tokenURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Add("x-csrf-token", csrf)
	res, err := executeRequest(req, client)
	if err != nil {
		return "", err
	}
	decoder := json.NewDecoder(res.Body)
	var tokens authProxyTokens
	err = decoder.Decode(&tokens)
	if err != nil {
		return "", err
	}
	return tokens.AccessToken, nil
}

func getCsrfToken(jar http.CookieJar) (string, error) {
	volkswagen, _ := url.Parse("https://www.volkswagen.de/")
	cookies := jar.Cookies(volkswagen)
	for _, c := range cookies {
		if c.Name == "csrf_token" {
			return c.Value, nil
		}
	}
	return "", fmt.Errorf("found no cookie with csrf token")
}

func executePostForm(url string, data url.Values, client http.Client) (*http.Response, error) {
	res, err := client.PostForm(url, data)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("status code error: %d %s", res.StatusCode, res.Status)
	}
	return res, nil
}

func executeRequest(req *http.Request, client http.Client) (*http.Response, error) {
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("status code error: %d %s", res.StatusCode, res.Status)
	}
	return res, nil
}

func getInputValues(body io.ReadCloser, ids []string) (map[string]string, error) {
	document, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, err
	}
	idValues := make(map[string]string)
	for _, id := range ids {
		value, find := document.Find(fmt.Sprintf("#%s", id)).Attr("value")
		if !find {
			return nil, fmt.Errorf("find no elememt with id %s", id)
		}
		idValues[id] = value
	}
	return idValues, nil
}
