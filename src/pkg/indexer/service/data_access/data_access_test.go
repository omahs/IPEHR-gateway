package data_access

import (
	"hms/gateway/pkg/storage"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"

	"hms/gateway/pkg/common/fake_data"
)

func TestDataAccessIndex(t *testing.T) {
	sc := &storage.StorageConfig{}
	sc.New("./test_" + strconv.FormatInt(time.Now().UnixNano(), 10))
	storage.Init(sc)

	dataAccessIndex := New()

	userUUID := uuid.New()
	userId := userUUID.String()

	accessGroupUUID := uuid.New()
	accessGroupId := accessGroupUUID.String()

	accessGroupKey, err := fake_data.GetByteArray(32)
	if err != nil {
		t.Fatal(err)
	}

	err = dataAccessIndex.Add(userId, accessGroupId, accessGroupKey)
	if err != nil {
		t.Fatal("dataAccessIndex add error:", err)
	}

	groupAccessKey, err := dataAccessIndex.Get(userId, accessGroupId)
	if err != nil {
		t.Fatal("dataAccessIndex get error:", err)
	}

	if len(groupAccessKey) < 32 {
		t.Fatal("groupAccessKey incorrect")
	}

}
