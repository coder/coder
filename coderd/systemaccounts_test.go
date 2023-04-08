package coderd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

func TestCreateSystemAccount(t *testing.T) {
	clearTable()

	api := &API{
		router:                mux.NewRouter(),
		systemAccountProvider: mockSystemAccountProvider{},
	}

	payload := []byte(`{"name": "Test Account"}`)
	req, _ := http.NewRequest("POST", "/api/v2/systemaccounts", bytes.NewBuffer(payload))
	rr := httptest.NewRecorder()

	handler := http.HandlerFunc(api.createSystemAccount)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("Expected response status code %d, but got %d", http.StatusCreated, rr.Code)
	}

	var res map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &res)

	if res["name"] != "Test Account" {
		t.Errorf("Expected account name to be 'Test Account', but got %v", res["name"])
	}

	if _, ok := res["id"].(string); !ok {
		t.Errorf("Expected 'id' field to be a string")
	}

	if _, ok := res["created_at"].(string); !ok {
		t.Errorf("Expected 'created_at' field to be a string")
	}

	if _, ok := res["updated_at"].(string); !ok {
		t.Errorf("Expected 'updated_at' field to be a string")
	}

	if _, ok := res["organization"].(string); !ok {
		t.Errorf("Expected 'organization' field to be a string")
	}

	if _, ok := res["created_by"].(string); !ok {
		t.Errorf("Expected 'created_by' field to be a string")
	}
}

func TestUpdateSystemAccount(t *testing.T) {
	clearTable()
	account := createTestAccount()
	payload := []byte(`{"name": "Updated Test Account"}`)
	req, _ := http.NewRequest("PUT", fmt.Sprintf("/api/v2/systemaccounts/%s", account.ID), bytes.NewBuffer(payload))
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(updateSystemAccount)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected response status code %d, but got %d", http.StatusOK, rr.Code)
	}

	var res map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &res)

	if res["name"] != "Updated Test Account" {
		t.Errorf("Expected account name to be 'Updated Test Account', but got %v", res["name"])
	}

	if _, ok := res["id"].(string); !ok {
		t.Errorf("Expected 'id' field to be a string")
	}

	if _, ok := res["created_at"].(string); !ok {
		t.Errorf("Expected 'created_at' field to be a string")
	}

	if _, ok := res["updated_at"].(string); !ok {
		t.Errorf("Expected 'updated_at' field to be a string")
	}

	if _, ok := res["organization"].(string); !ok {
		t.Errorf("Expected 'organization' field to be a string")
	}

	if _, ok := res["created_by"].(string); !ok {
		t.Errorf("Expected 'created_by' field to be a string")
	}
}

func TestDeleteSystemAccount(t *testing.T) {
	clearTable()
	account := createTestAccount()
	req, _ := http.NewRequest("DELETE", fmt.Sprintf("/api/v2/systemaccounts/%s", account.ID), nil)
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(deleteSystemAccount)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("Expected response status code %d, but got %d", http.StatusNoContent, rr.Code)
	}

	if accountExists(account.ID) {
		t.Errorf("Expected account to be deleted")
	}
}

func clearTable() {
	db.Exec("DELETE FROM system_accounts")
}

func createTestAccount() *SystemAccount {
	account := &SystemAccount{Name: "Test Account"}
	db.Exec("INSERT INTO system_accounts (id, name, created_at, updated_at) VALUES ($1, $2, $3, $4)", account.ID, account.Name, account.CreatedAt, account.UpdatedAt)
	return account
}

func accountExists(id uuid.UUID) bool {
	var count int64
	err := db.QueryRow("SELECT COUNT(*) FROM system_accounts WHERE id = $1", id).Scan(&count)
	if err != nil {
		log.Println(err)
		return false
	}
	return count > 0
}
