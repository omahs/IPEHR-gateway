package ehr

import (
	"context"
	"encoding/json"
	"fmt"
	"hms/gateway/pkg/indexer"
	"log"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/sha3"

	"hms/gateway/pkg/common"
	"hms/gateway/pkg/crypto/chachaPoly"
	"hms/gateway/pkg/crypto/keybox"
	"hms/gateway/pkg/docs/model"
	"hms/gateway/pkg/docs/model/base"
	"hms/gateway/pkg/docs/service"
	"hms/gateway/pkg/docs/service/processing"
	docStatus "hms/gateway/pkg/docs/status"
	"hms/gateway/pkg/docs/types"
	"hms/gateway/pkg/errors"
)

type Service struct {
	*service.DefaultDocumentService
}

func NewService(docService *service.DefaultDocumentService) *Service {
	return &Service{
		docService,
	}
}

func (s *Service) EhrCreate(ctx context.Context, userID string, ehrSystemID base.EhrSystemID, request *model.EhrCreateRequest) (*model.EHR, error) {
	ehrUUID := uuid.New()
	return s.EhrCreateWithID(ctx, userID, &ehrUUID, ehrSystemID, request)
}

func (s *Service) EhrCreateWithID(ctx context.Context, userID string, ehrUUID *uuid.UUID, ehrSystemID base.EhrSystemID, request *model.EhrCreateRequest) (*model.EHR, error) {
	var ehr model.EHR

	ehr.SystemID.Value = ehrSystemID.String()
	ehr.EhrID.Value = ehrUUID.String()

	ehr.EhrStatus.ID.Type = "OBJECT_VERSION_ID"
	ehr.EhrStatus.ID.Value = uuid.New().String() + "::" + ehrSystemID.String() + "::1"
	ehr.EhrStatus.Namespace = "local"
	ehr.EhrStatus.Type = "EHR_STATUS"

	ehr.EhrAccess.ID.Type = "OBJECT_VERSION_ID"
	ehr.EhrAccess.ID.Value = uuid.New().String() + "::" + ehrSystemID.String() + "::1"
	ehr.EhrAccess.Namespace = "local"
	ehr.EhrAccess.Type = "EHR_ACCESS"

	ehr.TimeCreated.Value = time.Now().Format(common.OpenEhrTimeFormat)

	// Creating EHR_STATUS
	ehrStatusID := ehr.EhrStatus.ID.Value
	subjectID := request.Subject.ExternalRef.ID.Value
	subjectNamespace := request.Subject.ExternalRef.Namespace

	doc, err := s.CreateStatus(ehrStatusID, subjectID, subjectNamespace)
	if err != nil {
		return nil, fmt.Errorf("create status error: %w", err)
	}

	var (
		reqID        = ctx.(*gin.Context).GetString("reqId")
		transactions = s.Infra.Index.MultiCallTxNew(s.Proc, processing.TxSetEhrUser, "", reqID)
	)

	err = s.SaveStatus(ctx, transactions, userID, ehrUUID, ehrSystemID, doc, true)
	if err != nil {
		return nil, fmt.Errorf("SaveStatus error: %w. ehrID: %s userID: %s", err, ehrUUID.String(), userID)
	}

	err = s.SaveEhr(ctx, transactions, userID, &ehr)
	if err != nil {
		return nil, fmt.Errorf("SaveEhr error: %w", err)
	}

	if err := transactions.Commit(); err != nil {
		return nil, fmt.Errorf("SaveEhr commit error: %w", err)
	}

	return &ehr, nil
}

func (s *Service) SaveEhr(ctx context.Context, transactions *indexer.MultiCallTx, userID string, doc *model.EHR) error {
	ehrUUID, err := uuid.Parse(doc.EhrID.Value)
	if err != nil {
		return fmt.Errorf("ehrUUID parse error: %w ehrID.Value %s", err, doc.EhrID.Value)
	}

	docBytes, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("ehr marshal error: %w", err)
	}

	if s.Infra.CompressionEnabled {
		docBytes, err = s.Infra.Compressor.Compress(docBytes)
		if err != nil {
			return fmt.Errorf("ehr compress error: %w", err)
		}
	}

	// Document encryption key generation
	key := chachaPoly.GenerateKey()

	// Document encryption
	docEncrypted, err := key.EncryptWithAuthData(docBytes, ehrUUID[:])
	if err != nil {
		return fmt.Errorf("ehr encryption error: %w", err)
	}

	// IPFS saving
	CID, err := s.Infra.IpfsClient.Add(ctx, docEncrypted)
	if err != nil {
		return fmt.Errorf("IpfsClient.Add error: %w", err)
	}

	// Filecoin saving
	dealCID, minerAddr, err := s.Infra.FilecoinClient.StartDeal(ctx, CID, uint64(len(docEncrypted)))
	if err != nil {
		return fmt.Errorf("FilecoinClient.StartDeal error: %w", err)
	}
	//dealCID := fakeData.Cid()
	//minerAddr := []byte("123")

	// Start processing request
	// TODO only one request ID should be in DB, checked it
	reqID := ctx.(*gin.Context).GetString("reqId") + "ehr"
	{
		procReq := &processing.Request{
			ReqID:        reqID,
			Kind:         processing.RequestEhrCreate,
			Status:       processing.StatusProcessing,
			UserID:       userID,
			EhrUUID:      ehrUUID.String(),
			CID:          CID.String(),
			DealCID:      dealCID.String(),
			MinerAddress: minerAddr,
		}
		if err = s.Proc.AddRequest(procReq); err != nil {
			return fmt.Errorf("Proc.AddRequest error: %w", err)
		}

		err = s.Proc.AddTx(reqID, dealCID.String(), "", processing.TxFilecoinStartDeal, processing.StatusPending)
		if err != nil {
			return fmt.Errorf("Proc.AddTx error: %w", err)
		}
	}

	// Index EHR userID -> docStorageID
	{
		packed, err := s.Infra.Index.SetEhrUser(userID, &ehrUUID)
		if err != nil {
			return fmt.Errorf("Index.SetEhrUser error: %w", err)
		}
		transactions.Add(packed)
	}

	// Index Docs ehr_id -> doc_meta
	{
		ehrIDEncrypted, err := key.EncryptWithAuthData(ehrUUID[:], ehrUUID[:])
		if err != nil {
			return fmt.Errorf("EncryptWithAuthData error: %w ehrID: %s", err, ehrUUID.String())
		}

		docMeta := &model.DocumentMeta{
			DocType:         uint8(types.Ehr),
			Status:          uint8(docStatus.ACTIVE),
			CID:             CID.Bytes(),
			DealCID:         dealCID.Bytes(),
			MinerAddress:    []byte(minerAddr),
			DocUIDEncrypted: ehrIDEncrypted,
			DocBaseUIDHash:  [32]byte{},
			IsLast:          true,
			Timestamp:       uint32(time.Now().Unix()),
		}

		packed, err := s.Infra.Index.AddEhrDoc(&ehrUUID, docMeta)
		if err != nil {
			return fmt.Errorf("Index.AddEhrDoc error: %w", err)
		}

		transactions.Add(packed)
	}

	// Index Access
	{
		userPubKey, _, err := s.Infra.Keystore.Get(userID)
		if err != nil {
			return fmt.Errorf("Keystore.Get error: %w userID %s", err, userID)
		}

		docAccessValue, err := keybox.SealAnonymous(key.Bytes(), userPubKey)
		if err != nil {
			return fmt.Errorf("keybox.SealAnonymous error: %w", err)
		}

		docAccessKey := sha3.Sum256(append(CID.Bytes()[:], []byte(userID)...))

		packed, err := s.Infra.Index.SetDocKeyEncrypted(&docAccessKey, docAccessValue)
		if err != nil {
			return fmt.Errorf("Index.SetDocAccess error: %w", err)
		}

		transactions.Add(packed)
	}

	return nil
}

// GetDocBySubject Get decrypted document by subject
func (s *Service) GetDocBySubject(ctx context.Context, userID, subjectID, namespace string) (docDecrypted []byte, err error) {
	ehrUUID, err := s.Infra.Index.GetEhrUUIDBySubject(ctx, subjectID, namespace)
	if err != nil {
		return nil, fmt.Errorf("Index.GetEhrUUIDBySubject error: %w. userID: %s subjectID: %s namespace: %s", err, userID, subjectID, namespace)
	}

	// Getting docStorageID
	docMeta, err := s.Infra.Index.GetDocLastByType(ctx, ehrUUID, types.Ehr)
	if err != nil {
		return nil, fmt.Errorf("Index.GetLastDocByType error: %w. ehrUUID: %s", err, ehrUUID.String())
	}

	// Getting doc from storage
	docDecrypted, err = s.GetDocFromStorageByID(ctx, userID, docMeta.Cid(), ehrUUID[:], docMeta.DocUIDEncrypted)
	if err != nil && errors.Is(err, errors.ErrIsInProcessing) {
		return nil, err
	} else if err != nil {
		return nil, fmt.Errorf("GetDocFromStorageByID error: %w. userID: %s, doc.CID: %x ehrUUID: %s", err, userID, docMeta.CID, ehrUUID.String())
	}

	return docDecrypted, nil
}

func (s *Service) CreateStatus(ehrStatusID, subjectID, subjectNamespace string) (doc *model.EhrStatus, err error) {
	doc = &model.EhrStatus{}
	doc.Type = types.EhrStatus.String()
	doc.ArchetypeNodeID = "openEHR-EHR-EHR_STATUS.generic.v1"
	doc.Name = base.DvText{Value: "EHR Status"}

	// todo FIXIT
	doc.UID = &base.UIDBasedID{ObjectID: base.ObjectID{
		Type:  "OBJECT_VERSION_ID",
		Value: ehrStatusID,
	}}

	doc.Subject.ExternalRef = base.ObjectRef{
		ID: base.ObjectID{
			Type:  "HIER_OBJECT_ID",
			Value: subjectID,
		},
		Namespace: subjectNamespace,
		Type:      "PERSON",
	}
	doc.IsQueryable = true
	doc.IsModifable = true

	return doc, nil
}

func (s *Service) UpdateEhr(ctx context.Context, multiCallTx *indexer.MultiCallTx, userID string, ehrUUID *uuid.UUID, status *model.EhrStatus) error {
	docMeta, err := s.Infra.Index.GetDocLastByType(ctx, ehrUUID, types.Ehr)
	if err != nil {
		return fmt.Errorf("Index.GetLastEhrDocByType error: %w. ehrID: %s", err, ehrUUID.String())
	}

	ehrDecrypted, err := s.GetDocFromStorageByID(ctx, userID, docMeta.Cid(), ehrUUID[:], docMeta.DocUIDEncrypted)
	if err != nil && errors.Is(err, errors.ErrIsInProcessing) {
		return err
	} else if err != nil {
		return fmt.Errorf("GetDocFromStorageByID error: %w. userID: %s StorageID: %x ehrID: %s", err, userID, docMeta.CID, ehrUUID.String())
	}

	var ehr model.EHR
	if err = json.Unmarshal(ehrDecrypted, &ehr); err != nil {
		return fmt.Errorf("ehr unmarshal error: %w", err)
	}

	if status.UID.Value != ehr.EhrStatus.ID.Value {
		ehr.EhrStatus.ID.Value = status.UID.Value
		if err = s.SaveEhr(ctx, multiCallTx, userID, &ehr); err != nil {
			return fmt.Errorf("ehr save error: %w", err)
		}
	}

	return nil
}

func (s *Service) SaveStatus(ctx context.Context, multiCallTx *indexer.MultiCallTx, userID string, ehrUUID *uuid.UUID, ehrSystemID base.EhrSystemID, status *model.EhrStatus, isNew bool) error {
	// Document encryption key generation
	key := chachaPoly.GenerateKey()

	objectVersionID, err := base.NewObjectVersionID(status.UID.Value, ehrSystemID)
	if err != nil {
		return fmt.Errorf("SaveStatus error: %w versionUID %s ehrSystemID %s", err, objectVersionID.String(), ehrSystemID.String())
	}

	baseDocumentUID := []byte(objectVersionID.BasedID())
	baseDocumentUIDHash := sha3.Sum256(baseDocumentUID)

	statusBytes, err := json.Marshal(status)
	if err != nil {
		return fmt.Errorf("json.Marshal error: %w", err)
	}

	if s.Infra.CompressionEnabled {
		statusBytes, err = s.Infra.Compressor.Compress(statusBytes)
		if err != nil {
			return fmt.Errorf("Compress error: %w", err)
		}
	}

	// Document encryption
	statusEncrypted, err := key.EncryptWithAuthData(statusBytes, []byte(status.UID.Value))
	if err != nil {
		return fmt.Errorf("EncryptWithAuthData error: %w", err)
	}

	// IPFS saving
	CID, err := s.Infra.IpfsClient.Add(ctx, statusEncrypted)
	if err != nil {
		return fmt.Errorf("IpfsClient.Add error: %w", err)
	}

	// Filecoin saving
	dealCID, minerAddr, err := s.Infra.FilecoinClient.StartDeal(ctx, CID, uint64(len(statusEncrypted)))
	if err != nil {
		return fmt.Errorf("FilecoinClient.StartDeal error: %w", err)
	}
	//dealCID := fakeData.Cid()
	//minerAddr := []byte("123")

	requestKind := processing.RequestEhrStatusCreate
	if !isNew {
		requestKind = processing.RequestEhrStatusUpdate
	}

	// Start processing request
	reqID := ctx.(*gin.Context).GetString("reqId")
	{
		procReq := &processing.Request{
			ReqID:        reqID,
			Kind:         requestKind,
			UserID:       userID,
			EhrUUID:      ehrUUID.String(),
			Status:       processing.StatusProcessing,
			CID:          CID.String(),
			DealCID:      dealCID.String(),
			MinerAddress: minerAddr,
		}
		err = s.Proc.AddRequest(procReq)
		if err != nil {
			return fmt.Errorf("Proc.AddRequest error: %w", err)
		}

		err = s.Proc.AddTx(reqID, dealCID.String(), "", processing.TxFilecoinStartDeal, processing.StatusPending)
		if err != nil {
			return fmt.Errorf("Proc.AddTx error: %w", err)
		}
	}

	// Index subject and namespace
	{
		subjectID := status.Subject.ExternalRef.ID.Value
		subjectNamespace := status.Subject.ExternalRef.Namespace

		setSubjectPacked, err := s.Infra.Index.SetSubject(ehrUUID, subjectID, subjectNamespace)
		if err != nil {
			return fmt.Errorf("Index.SetSubject error: %w ehrID: %s subjectID: %s subjectNamespace: %s", err, ehrUUID.String(), subjectID, subjectNamespace)
		}

		multiCallTx.Add(setSubjectPacked)
	}

	// Index Docs ehr_id -> doc_meta
	{
		statusIDEncrypted, err := key.EncryptWithAuthData([]byte(objectVersionID.String()), ehrUUID[:])
		if err != nil {
			return fmt.Errorf("EncryptWithAuthData error: %w ehrID: %s statusUid: %s", err, ehrUUID.String(), status.UID.Value)
		}

		docMeta := &model.DocumentMeta{
			DocType:         uint8(types.EhrStatus),
			Status:          uint8(docStatus.ACTIVE),
			CID:             CID.Bytes(),
			DealCID:         dealCID.Bytes(),
			MinerAddress:    []byte(minerAddr),
			DocUIDEncrypted: statusIDEncrypted,
			DocBaseUIDHash:  baseDocumentUIDHash,
			Version:         *objectVersionID.VersionBytes(),
			IsLast:          true,
			Timestamp:       uint32(time.Now().Unix()),
		}

		packed, err := s.Infra.Index.AddEhrDoc(ehrUUID, docMeta)
		if err != nil {
			return fmt.Errorf("Index.AddEhrDoc error: %w", err)
		}
		multiCallTx.Add(packed)
	}

	// Index Access
	{
		userPubKey, _, err := s.Infra.Keystore.Get(userID)
		if err != nil {
			return fmt.Errorf("Keystore.Get error: %w userID %s", err, userID)
		}

		docAccessValue, err := keybox.SealAnonymous(key.Bytes(), userPubKey)
		if err != nil {
			return fmt.Errorf("keybox.SealAnonymous error: %w", err)
		}

		docAccessKey := sha3.Sum256(append(CID.Bytes()[:], []byte(userID)...))

		packed, err := s.Infra.Index.SetDocKeyEncrypted(&docAccessKey, docAccessValue)
		if err != nil {
			return fmt.Errorf("Index.SetDocAccess error: %w", err)
		}
		multiCallTx.Add(packed)
	}

	return nil
}

func (s *Service) UpdateStatus(ctx context.Context, userID string, ehrUUID *uuid.UUID, ehrSystemID base.EhrSystemID, status *model.EhrStatus) error {
	reqID := ctx.(*gin.Context).GetString("reqId")

	var transactions = s.Infra.Index.MultiCallTxNew(s.Proc, processing.TxSetEhrUser, "UpdateEhrStatus", reqID)

	if err := s.SaveStatus(ctx, transactions, userID, ehrUUID, ehrSystemID, status, false); err != nil {
		log.Println("SaveStatus error:", err)
		return errors.New("EHR_STATUS saving error")
	}

	// TODO i dont like this logic, because in method GetByID we always grab whole data from filecoin, which contain last status id. It need fix it.
	if err := s.UpdateEhr(ctx, transactions, userID, ehrUUID, status); err != nil {
		log.Println("UpdateEhr error:", err)
		return errors.New("EHR updating error")
	}

	if err := transactions.Commit(); err != nil {
		return fmt.Errorf("UpdateStatus commit error: %w", err)
	}

	return nil
}

// GetStatus Get current (last) status of EHR document
func (s *Service) GetStatus(ctx context.Context, userID string, ehrUUID *uuid.UUID) (*model.EhrStatus, error) {
	docMeta, err := s.Infra.Index.GetDocLastByType(ctx, ehrUUID, types.EhrStatus)
	if err != nil {
		return nil, fmt.Errorf("Index.GetLastEhrDocByType error: %w. ehrID: %s", err, ehrUUID.String())
	}

	docDecrypted, err := s.GetDocFromStorageByID(ctx, userID, docMeta.Cid(), ehrUUID[:], docMeta.DocUIDEncrypted)
	if err != nil && errors.Is(err, errors.ErrIsInProcessing) {
		return nil, err
	} else if err != nil {
		return nil, fmt.Errorf("GetDocFromStorageByID error: %w", err)
	}

	var status model.EhrStatus
	if err := json.Unmarshal(docDecrypted, &status); err != nil {
		return nil, fmt.Errorf("EHR status unmarshal error: %w", err)
	}

	return &status, nil
}

func (s *Service) GetStatusByNearestTime(ctx context.Context, userID string, ehrUUID *uuid.UUID, nearestTime time.Time, docType types.DocumentType) (*model.EhrStatus, error) {
	docMeta, err := s.Infra.Index.GetDocByTime(ctx, ehrUUID, types.EhrStatus, uint32(nearestTime.Unix()))
	if err != nil {
		return nil, fmt.Errorf("DocsIndex.GetByNearestTime error: %w ehrID %s nearestTime %s docType %s", err, ehrUUID.String(), nearestTime.String(), docType.String())
	}

	docDecrypted, err := s.GetDocFromStorageByID(ctx, userID, docMeta.Cid(), ehrUUID[:], docMeta.DocUIDEncrypted)
	if err != nil && errors.Is(err, errors.ErrIsInProcessing) {
		return nil, err
	} else if err != nil {
		return nil, fmt.Errorf("GetDocFromStorageByID error: %w", err)
	}

	var status model.EhrStatus
	if err := json.Unmarshal(docDecrypted, &status); err != nil {
		return nil, fmt.Errorf("EHR status unmarshal error: %w", err)
	}

	return &status, nil
}

func (s *Service) ValidateEhr(ehr *model.EHR) bool {
	// TODO
	return true
}

func (s *Service) ValidateStatus(status *model.EhrStatus) bool {
	// TODO
	return true
}
