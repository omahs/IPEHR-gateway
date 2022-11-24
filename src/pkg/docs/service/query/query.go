package query

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"hms/gateway/pkg/common"
	"hms/gateway/pkg/compressor"
	"hms/gateway/pkg/crypto/chachaPoly"
	"hms/gateway/pkg/crypto/keybox"
	"hms/gateway/pkg/docs/model"
	"hms/gateway/pkg/docs/service"
	"hms/gateway/pkg/docs/service/processing"
	"hms/gateway/pkg/docs/status"
	"hms/gateway/pkg/docs/types"
	"hms/gateway/pkg/errors"
	"hms/gateway/pkg/indexer/ehrIndexer"

	"github.com/vmihailenco/msgpack/v5"
	"golang.org/x/crypto/sha3"
)

const defaultVersion = "1.0.1"

type Service struct {
	*service.DefaultDocumentService
}

func NewService(docService *service.DefaultDocumentService) *Service {
	return &Service{
		docService,
	}
}

func (*Service) List(ctx context.Context, userID, qualifiedQueryName string) ([]*model.StoredQuery, error) {
	return nil, nil
}

func (*Service) Validate(data []byte) bool {
	return true
}

func (s *Service) Store(ctx context.Context, userID, systemID, reqID, qType, name, q string) (*model.StoredQuery, error) {
	return s.StoreVersion(ctx, userID, systemID, reqID, qType, name, defaultVersion, q)
}

func (s *Service) StoreVersion(ctx context.Context, userID, systemID, reqID, qType, name, version, q string) (*model.StoredQuery, error) {
	timestamp := time.Now()

	storedQuery := &model.StoredQuery{
		Name:        name,
		Type:        qType,
		Version:     version,
		TimeCreated: timestamp.Format(common.OpenEhrTimeFormat),
		Query:       q,
	}

	id := []byte(userID + systemID + storedQuery.Name + storedQuery.Version)
	idHash := sha3.Sum256(id)
	key := chachaPoly.GenerateKey()

	content, err := msgpack.Marshal(storedQuery)
	if err != nil {
		return nil, fmt.Errorf("msgpack.Marshal error: %w", err)
	}

	contentCompresed, err := compressor.New(compressor.BestCompression).Compress(content)
	if err != nil {
		return nil, fmt.Errorf("Query Compress error: %w", err)
	}

	contentEncr, err := key.Encrypt(contentCompresed)
	if err != nil {
		return nil, fmt.Errorf("key.Encrypt content error: %w", err)
	}

	log.Printf("contentEncr: %x", contentEncr)

	userPubKey, userPrivKey, err := s.Infra.Keystore.Get(userID)
	if err != nil {
		return nil, fmt.Errorf("Keystore.Get error: %w userID %s", err, userID)
	}

	keyEncr, err := keybox.Seal(key.Bytes(), userPubKey, userPrivKey)
	if err != nil {
		return nil, fmt.Errorf("keybox.SealAnonymous error: %w", err)
	}

	docMeta := &model.DocumentMeta{
		Status:    uint8(status.ACTIVE),
		Id:        idHash[:],
		Version:   nil,
		Timestamp: uint32(timestamp.Unix()),
		IsLast:    true,
		Attrs: []ehrIndexer.AttributesAttribute{
			{Code: model.AttributeKeyEncr, Value: keyEncr},         // encrypted with key
			{Code: model.AttributeContentEncr, Value: contentEncr}, // encrypted with userPubKey
		},
	}

	packed, err := s.Infra.Index.AddEhrDoc(ctx, types.Query, docMeta, userPrivKey, nil)
	if err != nil {
		return nil, fmt.Errorf("Index.AddEhrDoc error: %w", err)
	}

	procRequest, err := s.Proc.NewRequest(reqID, userID, "", processing.RequestQueryStore)
	if err != nil {
		return nil, fmt.Errorf("Proc.NewRequest error: %w", err)
	}

	txHash, err := s.Infra.Index.SendSingle(ctx, packed)
	if err != nil {
		if strings.Contains(err.Error(), "NFD") {
			return nil, errors.ErrNotFound
		} else if strings.Contains(err.Error(), "AEX") {
			return nil, errors.ErrAlreadyExist
		}

		return nil, fmt.Errorf("Index.SendSingle error: %w", err)
	}

	procRequest.AddEthereumTx(processing.TxAddEhrDoc, txHash)

	if err := procRequest.Commit(); err != nil {
		return nil, fmt.Errorf("EHR create procRequest commit error: %w", err)
	}

	return storedQuery, nil
}
