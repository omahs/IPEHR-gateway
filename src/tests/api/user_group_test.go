package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"hms/gateway/pkg/common/fakeData"
	"hms/gateway/pkg/errors"
	"hms/gateway/pkg/user/model"
)

func (testWrap *testWrap) userGroupCreate(testData *TestData) func(t *testing.T) {
	return func(t *testing.T) {
		err := testWrap.checkUser(testData)
		if err != nil {
			t.Fatal(err)
		}

		user := testData.users[0]

		if user.accessToken == "" {
			err := user.login(testData.ehrSystemID, testWrap.server.URL, testWrap.httpClient)
			if err != nil {
				t.Fatal(err)
			}
		}

		name := fakeData.GetRandomStringWithLength(10)
		description := fakeData.GetRandomStringWithLength(10)

		userGroup, _, err := userGroupCreate(user, testData.ehrSystemID, testWrap.server.URL, name, description, testWrap.httpClient)
		if err != nil {
			t.Fatal(err)
		}

		testData.userGroups = append(testData.userGroups, userGroup)
	}
}

func (testWrap *testWrap) userGroupAddUser(testData *TestData) func(t *testing.T) {
	return func(t *testing.T) {
		err := testWrap.checkUser(testData)
		if err != nil {
			t.Fatal(err)
		}

		user := testData.users[0]

		if user.accessToken == "" {
			err := user.login(testData.ehrSystemID, testWrap.server.URL, testWrap.httpClient)
			if err != nil {
				t.Fatal(err)
			}
		}

		err = checkUserGroup(user, testData, testWrap.server.URL, testWrap.httpClient)
		if err != nil {
			t.Fatal("checkUserGroup error: ", err)
		}

		userGroup := testData.userGroups[0]

		url := testWrap.server.URL + "/v1/user/group/" + userGroup.GroupID.String() + "/user_add/" + user.id + "/admin"

		request, err := http.NewRequest(http.MethodPut, url, nil)
		if err != nil {
			t.Fatal(err)
		}

		request.Header.Set("AuthUserId", user.id)
		request.Header.Set("Authorization", "Bearer "+user.accessToken)
		request.Header.Set("EhrSystemId", testData.ehrSystemID)

		response, err := testWrap.httpClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()

		data, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}

		if response.StatusCode != http.StatusOK {
			t.Fatalf("Expected: %d, received: %d, body: %s", http.StatusOK, response.StatusCode, data)
		}
	}
}

func (testWrap *testWrap) userGroupGetByID(testData *TestData) func(t *testing.T) {
	return func(t *testing.T) {
		err := testWrap.checkUser(testData)
		if err != nil {
			t.Fatal(err)
		}

		user := testData.users[0]

		if user.accessToken == "" {
			err := user.login(testData.ehrSystemID, testWrap.server.URL, testWrap.httpClient)
			if err != nil {
				t.Fatal(err)
			}
		}

		err = checkUserGroup(user, testData, testWrap.server.URL, testWrap.httpClient)
		if err != nil {
			t.Fatal("checkUserGroup error: ", err)
		}

		userGroup1 := testData.userGroups[0]

		url := testWrap.server.URL + "/v1/user/group/" + userGroup1.GroupID.String()

		request, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			t.Fatal(err)
		}

		request.Header.Set("AuthUserId", user.id)
		request.Header.Set("Authorization", "Bearer "+user.accessToken)
		request.Header.Set("EhrSystemId", testData.ehrSystemID)

		response, err := testWrap.httpClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()

		data, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}

		if response.StatusCode != http.StatusOK {
			t.Fatalf("Expected: %d, received: %d, body: %s", http.StatusOK, response.StatusCode, data)
		}

		var userGroup model.UserGroup

		err = json.Unmarshal(data, &userGroup)
		if err != nil {
			t.Fatal(err)
		}

		if userGroup.GroupID.String() != userGroup1.GroupID.String() {
			t.Fatalf("Expected UUID: %s, received: %s", userGroup1.GroupID, userGroup.GroupID)
		}
	}
}

func (testWrap *testWrap) userGroupGetList(testData *TestData) func(t *testing.T) {
	return func(t *testing.T) {
		err := testWrap.checkUser(testData)
		if err != nil {
			t.Fatal(err)
		}

		user := testData.users[0]

		if user.accessToken == "" {
			err := user.login(testData.ehrSystemID, testWrap.server.URL, testWrap.httpClient)
			if err != nil {
				t.Fatal(err)
			}
		}

		err = checkUserGroup(user, testData, testWrap.server.URL, testWrap.httpClient)
		if err != nil {
			t.Fatal("checkUserGroup error: ", err)
		}

		url := testWrap.server.URL + "/v1/user/group"

		request, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			t.Fatal(err)
		}

		request.Header.Set("AuthUserId", user.id)
		request.Header.Set("Authorization", "Bearer "+user.accessToken)
		request.Header.Set("EhrSystemId", testData.ehrSystemID)

		response, err := testWrap.httpClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()

		data, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}

		if response.StatusCode != http.StatusOK {
			t.Fatalf("Expected: %d, received: %d, body: %s", http.StatusOK, response.StatusCode, data)
		}

		var userGroupList []model.UserGroup

		err = json.Unmarshal(data, &userGroupList)
		if err != nil {
			t.Fatal(err)
		}

		if len(userGroupList) == 0 {
			t.Fatalf("Expected: userGroups, received: empty, body: %s", data)
		}
	}
}

func userGroupCreate(user *User, systemID, baseURL, name, description string, client *http.Client) (*model.UserGroup, string, error) {
	userGroup := &model.UserGroup{
		Name:        name,
		Description: description,
	}

	data, _ := json.Marshal(userGroup)
	url := baseURL + "/v1/user/group"

	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, "", err
	}

	request.Header.Set("AuthUserId", user.id)
	request.Header.Set("Authorization", "Bearer "+user.accessToken)
	request.Header.Set("EhrSystemId", systemID)

	response, err := client.Do(request)
	if err != nil {
		return nil, "", err
	}
	defer response.Body.Close()

	data, err = io.ReadAll(response.Body)
	if err != nil {
		return nil, "", err
	}

	if response.StatusCode != http.StatusCreated {
		if response.StatusCode == http.StatusConflict {
			return nil, "", errors.ErrAlreadyExist
		}

		return nil, "", errors.New(response.Status + " data: " + string(data))
	}

	var userGroup2 model.UserGroup

	err = json.Unmarshal(data, &userGroup2)
	if err != nil {
		return nil, "", err
	}

	if userGroup2.Name != userGroup.Name {
		return nil, "", errors.ErrFieldIsIncorrect("Name")
	}

	requestID := response.Header.Get("RequestId")

	return &userGroup2, requestID, nil
}

func checkUserGroup(user *User, testData *TestData, baseURL string, client *http.Client) error {
	if len(testData.userGroups) == 0 {
		name := fakeData.GetRandomStringWithLength(10)
		description := fakeData.GetRandomStringWithLength(10)

		userGroup, reqID, err := userGroupCreate(user, testData.ehrSystemID, baseURL, name, description, client)
		if err != nil {
			return fmt.Errorf("userGroupCreate error: %w", err)
		}

		err = requestWait(user.id, user.accessToken, reqID, baseURL, client)
		if err != nil {
			return fmt.Errorf("requestWait error, err: %w", err)
		}

		testData.userGroups = append(testData.userGroups, userGroup)
	}

	return nil
}
