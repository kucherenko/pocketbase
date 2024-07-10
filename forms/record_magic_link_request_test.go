package forms_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/forms"
	"github.com/pocketbase/pocketbase/models"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/types"
)

func TestRecordMagicLinkRequestSubmit(t *testing.T) {
	t.Parallel()

	testApp, _ := tests.NewTestApp()
	defer testApp.Cleanup()

	authCollection, err := testApp.Dao().FindCollectionByNameOrId("clients")
	if err != nil {
		t.Fatal(err)
	}

	scenarios := []struct {
		jsonData    string
		expectError bool
		expectMail  bool
	}{
		// empty field (Validate call check)
		{
			`{"email":""}`,
			true,
			false,
		},
		// invalid email field (Validate call check)
		{
			`{"email":"invalid"}`,
			true,
			false,
		},
		// nonexisting user
		{
			`{"email":"missing@example.com"}`,
			true,
			false,
		},
		// existing user (already verified)
		{
			`{"email":"test@example.com"}`,
			false,
			false,
		},
		// existing user (already verified) - repeating request to test threshod skip
		{
			`{"email":"test@example.com"}`,
			false,
			false,
		},
		// existing user (unverified)
		{
			`{"email":"test2@example.com"}`,
			false,
			true,
		},
		// existing user (inverified) - reached send threshod
		{
			`{"email":"test2@example.com"}`,
			true,
			false,
		},
	}

	now := types.NowDateTime()
	time.Sleep(1 * time.Millisecond)

	for i, s := range scenarios {
		testApp.TestMailer.TotalSend = 0 // reset
		form := forms.NewRecordMagicLinkRequest(testApp, authCollection)

		// load data
		loadErr := json.Unmarshal([]byte(s.jsonData), form)
		if loadErr != nil {
			t.Errorf("[%d] Failed to load form data: %v", i, loadErr)
			continue
		}

		interceptorCalls := 0
		interceptor := func(next forms.InterceptorNextFunc[*models.Record]) forms.InterceptorNextFunc[*models.Record] {
			return func(r *models.Record) error {
				interceptorCalls++
				return next(r)
			}
		}

		err := form.Submit(interceptor)

		// check interceptor calls
		expectInterceptorCalls := 1
		if s.expectError {
			expectInterceptorCalls = 0
		}
		if interceptorCalls != expectInterceptorCalls {
			t.Errorf("[%d] Expected interceptor to be called %d, got %d", i, expectInterceptorCalls, interceptorCalls)
		}

		hasErr := err != nil
		if hasErr != s.expectError {
			t.Errorf("[%d] Expected hasErr to be %v, got %v (%v)", i, s.expectError, hasErr, err)
		}

		expectedMails := 0
		if s.expectMail {
			expectedMails = 1
		}
		if testApp.TestMailer.TotalSend != expectedMails {
			t.Errorf("[%d] Expected %d mail(s) to be sent, got %d", i, expectedMails, testApp.TestMailer.TotalSend)
		}

		if s.expectError {
			continue
		}

		user, err := testApp.Dao().FindAuthRecordByEmail(authCollection.Id, form.Email)
		if err != nil {
			t.Errorf("[%d] Expected user with email %q to exist, got nil", i, form.Email)
			continue
		}

		// check whether LastMagicLinkSentAt was updated
		if !user.Verified() && user.LastMagicLinkSentAt().Time().Sub(now.Time()) < 0 {
			t.Errorf("[%d] Expected LastMagicLinkSentAt to be after %v, got %v", i, now, user.LastMagicLinkSentAt())
		}
	}
}

func TestRecordMagicLinkRequestInterceptors(t *testing.T) {
	t.Parallel()

	testApp, _ := tests.NewTestApp()
	defer testApp.Cleanup()

	authCollection, err := testApp.Dao().FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}

	authRecord, err := testApp.Dao().FindAuthRecordByEmail("users", "test@example.com")
	if err != nil {
		t.Fatal(err)
	}

	form := forms.NewRecordMagicLinkRequest(testApp, authCollection)
	form.Email = authRecord.Email()
	interceptorLastMagicLinkSentAt := authRecord.LastMagicLinkSentAt()
	testErr := errors.New("test_error")

	interceptor1Called := false
	interceptor1 := func(next forms.InterceptorNextFunc[*models.Record]) forms.InterceptorNextFunc[*models.Record] {
		return func(record *models.Record) error {
			interceptor1Called = true
			return next(record)
		}
	}

	interceptor2Called := false
	interceptor2 := func(next forms.InterceptorNextFunc[*models.Record]) forms.InterceptorNextFunc[*models.Record] {
		return func(record *models.Record) error {
			interceptorLastMagicLinkSentAt = record.LastMagicLinkSentAt()
			interceptor2Called = true
			return testErr
		}
	}

	submitErr := form.Submit(interceptor1, interceptor2)
	if submitErr != testErr {
		t.Fatalf("Expected submitError %v, got %v", testErr, submitErr)
	}

	if !interceptor1Called {
		t.Fatalf("Expected interceptor1 to be called")
	}

	if !interceptor2Called {
		t.Fatalf("Expected interceptor2 to be called")
	}

	if interceptorLastMagicLinkSentAt.String() != authRecord.LastMagicLinkSentAt().String() {
		t.Fatalf("Expected the form model to NOT be filled before calling the interceptors")
	}
}
