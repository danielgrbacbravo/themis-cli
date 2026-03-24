package login

import (
	"errors"
	"testing"
)

func TestNewModel_PrefillSavePasswordImpliesSaveUsername(t *testing.T) {
	m := newModel(Prefill{
		Username:     "s1234567",
		Password:     "secret",
		SaveUsername: false,
		SavePassword: true,
	}, nil)

	if !m.saveUsername {
		t.Fatalf("expected saveUsername=true when savePassword=true")
	}
	if !m.savePassword {
		t.Fatalf("expected savePassword=true")
	}
	if got := m.usernameInput.Value(); got != "s1234567" {
		t.Fatalf("unexpected prefilled username: got %q", got)
	}
	if got := m.passwordInput.Value(); got != "secret" {
		t.Fatalf("unexpected prefilled password: got %q", got)
	}
}

func TestStartSubmit_ValidatesRequiredFields(t *testing.T) {
	called := false
	m := newModel(Prefill{}, func(req SubmitRequest) (SubmitResult, error) {
		called = true
		return SubmitResult{}, nil
	})
	m.focusIndex = 5

	out, cmd := m.startSubmit()
	next := out.(model)

	if called {
		t.Fatalf("submit should not be called with empty required fields")
	}
	if cmd != nil {
		t.Fatalf("expected nil command for validation failure")
	}
	if next.submitting {
		t.Fatalf("expected submitting=false on validation failure")
	}
	if next.statusLine == "" {
		t.Fatalf("expected validation error status")
	}
}

func TestStartSubmit_SuccessAndFailureMessages(t *testing.T) {
	submitErr := errors.New("bad credentials")
	m := newModel(Prefill{}, func(req SubmitRequest) (SubmitResult, error) {
		if req.TOTP == "111111" {
			return SubmitResult{}, submitErr
		}
		return SubmitResult{UserFullName: "Jane Doe", UserEmail: "jane@example.com"}, nil
	})
	m.focusIndex = 5
	m.usernameInput.SetValue("s1234567")
	m.passwordInput.SetValue("secret")
	m.totpInput.SetValue("111111")

	out, cmd := m.startSubmit()
	next := out.(model)
	if cmd == nil {
		t.Fatalf("expected async submit cmd")
	}
	if !next.submitting {
		t.Fatalf("expected submitting=true after submit start")
	}

	msg := cmd().(submitFinishedMsg)
	updated, _ := next.Update(msg)
	afterFail := updated.(model)
	if afterFail.submitting {
		t.Fatalf("expected submitting=false after submit finishes")
	}
	if afterFail.submitErr == nil {
		t.Fatalf("expected submitErr to be recorded")
	}
	if afterFail.statusLine == "" {
		t.Fatalf("expected failure status line")
	}

	afterFail.totpInput.SetValue("222222")
	out, cmd = afterFail.startSubmit()
	next = out.(model)
	if cmd == nil {
		t.Fatalf("expected cmd for retry submit")
	}
	msg = cmd().(submitFinishedMsg)
	updated, _ = next.Update(msg)
	afterSuccess := updated.(model)
	if afterSuccess.submitErr != nil {
		t.Fatalf("expected submitErr cleared on success, got %v", afterSuccess.submitErr)
	}
	if afterSuccess.submitResult.UserEmail != "jane@example.com" {
		t.Fatalf("unexpected success result: %+v", afterSuccess.submitResult)
	}
	if afterSuccess.statusLine == "" {
		t.Fatalf("expected success status line")
	}
}
