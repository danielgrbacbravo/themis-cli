package themis

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func TestNextAuthAction_CredentialsForm(t *testing.T) {
	doc := mustDoc(t, `
<html><body>
  <form action="/sso/submit" method="post">
    <input type="hidden" name="state" value="abc" />
    <input type="text" name="Ecom_User_ID" value="" />
    <input type="password" name="Ecom_Password" value="" />
  </form>
</body></html>`)
	pageURL, _ := url.Parse("https://signon.rug.nl/start")

	next, stage, err := nextAuthAction(doc, pageURL, LoginRequest{
		Username: "alice",
		Password: "secret",
		TOTP:     "123456",
	})
	if err != nil {
		t.Fatalf("next action error: %v", err)
	}
	if stage != "credentials" {
		t.Fatalf("unexpected stage: %q", stage)
	}
	if next.method != http.MethodPost {
		t.Fatalf("unexpected method: %q", next.method)
	}
	if next.targetURL != "https://signon.rug.nl/sso/submit" {
		t.Fatalf("unexpected target: %q", next.targetURL)
	}
	if got := next.values.Get("Ecom_User_ID"); got != "alice" {
		t.Fatalf("unexpected username value: %q", got)
	}
	if got := next.values.Get("Ecom_Password"); got != "secret" {
		t.Fatalf("unexpected password value: %q", got)
	}
	if got := next.values.Get("state"); got != "abc" {
		t.Fatalf("unexpected hidden field: %q", got)
	}
}

func TestNextAuthAction_MFAForm(t *testing.T) {
	doc := mustDoc(t, `
<html><body>
  <form action="/mfa" method="post">
    <input type="hidden" name="flow" value="1" />
    <input type="text" name="nffc" value="" />
  </form>
</body></html>`)
	pageURL, _ := url.Parse("https://xfactor.rug.nl/challenge")

	next, stage, err := nextAuthAction(doc, pageURL, LoginRequest{
		Username: "alice",
		Password: "secret",
		TOTP:     "654321",
	})
	if err != nil {
		t.Fatalf("next action error: %v", err)
	}
	if stage != "mfa" {
		t.Fatalf("unexpected stage: %q", stage)
	}
	if got := next.values.Get("nffc"); got != "654321" {
		t.Fatalf("unexpected mfa value: %q", got)
	}
}

func TestBuildFormRequest_RelativeAction(t *testing.T) {
	doc := mustDoc(t, `
<form action="../submit" method="post">
  <input type="hidden" name="foo" value="bar" />
</form>`)
	form := doc.Find("form").First()
	pageURL, _ := url.Parse("https://connect.surfconext.nl/auth/step")

	targetURL, values, err := buildFormRequest(form, pageURL)
	if err != nil {
		t.Fatalf("build form request error: %v", err)
	}
	if targetURL.String() != "https://connect.surfconext.nl/submit" {
		t.Fatalf("unexpected target URL: %q", targetURL.String())
	}
	if values.Get("foo") != "bar" {
		t.Fatalf("unexpected values: %#v", values)
	}
}

func mustDoc(t *testing.T, html string) *goquery.Document {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatalf("parse doc: %v", err)
	}
	return doc
}
