package ehr

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/ipfs/go-cid"
	"golang.org/x/crypto/sha3"

	"hms/gateway/pkg/common"
	"hms/gateway/pkg/crypto/chachaPoly"
	"hms/gateway/pkg/crypto/keybox"
	"hms/gateway/pkg/docs/model"
	"hms/gateway/pkg/docs/model/base"
	"hms/gateway/pkg/docs/service"
	proc "hms/gateway/pkg/docs/service/processing"
	docStatus "hms/gateway/pkg/docs/status"
	"hms/gateway/pkg/docs/types"
	"hms/gateway/pkg/errors"
	"hms/gateway/pkg/indexer"
	"hms/gateway/pkg/indexer/ehrIndexer"
)

type Service struct {
	*service.DefaultDocumentService
}

func NewService(docService *service.DefaultDocumentService) *Service {
	return &Service{
		docService,
	}
}

func (s *Service) EhrCreate(ctx context.Context, userID, systemID string, ehrUUID *uuid.UUID, request *model.EhrCreateRequest, procRequest *proc.Request) (*model.EHR, error) {
	return s.EhrCreateWithID(ctx, userID, systemID, ehrUUID, request, procRequest)
}

func (s *Service) EhrCreateWithID(ctx context.Context, userID, systemID string, ehrUUID *uuid.UUID, request *model.EhrCreateRequest, procRequest *proc.Request) (*model.EHR, error) {
	var ehr model.EHR

	ehr.SystemID.Value = systemID
	ehr.EhrID.Value = ehrUUID.String()

	ehr.EhrAccess.ID.Type = "OBJECT_VERSION_ID"
	ehr.EhrAccess.ID.Value = uuid.New().String() + "::" + systemID + "::1"
	ehr.EhrAccess.Namespace = "local"
	ehr.EhrAccess.Type = "EHR_ACCESS"

	ehr.TimeCreated.Value = time.Now().Format(common.OpenEhrTimeFormat)

	// Creating EHR_STATUS
	ehrStatusID := uuid.New().String() + "::" + systemID + "::1"
	subjectID := request.Subject.ExternalRef.ID.Value
	subjectNamespace := request.Subject.ExternalRef.Namespace

	subject := s.CreateSubject(subjectID, subjectNamespace, "PERSON")

	doc, err := s.CreateStatus(ehrStatusID, subject)
	if err != nil {
		return nil, fmt.Errorf("create status error: %w", err)
	}

	_, userPrivKey, err := s.Infra.Keystore.Get(userID)
	if err != nil {
		return nil, fmt.Errorf("Keystore.Get error: %w userID %s", err, userID)
	}

	multiCallTx, err := s.Infra.Index.MultiCallEhrNew(ctx, userPrivKey)
	if err != nil {
		return nil, fmt.Errorf("MultiCallEhrNew error: %w. userID: %s", err, userID)
	}

	// Index EHR userIDHash -> ehrUUID
	{
		packed, err := s.Infra.Index.SetEhrUser(ctx, userID, systemID, ehrUUID, userPrivKey, multiCallTx.Nonce())
		if err != nil {
			return nil, fmt.Errorf("Index.SetEhrUser error: %w", err)
		}

		multiCallTx.Add(uint8(proc.TxSetEhrUser), packed)
	}

	err = s.SaveStatus(ctx, multiCallTx, procRequest, userID, systemID, ehrUUID, doc)
	if err != nil {
		return nil, fmt.Errorf("SaveStatus error: %w. ehrID: %s userID: %s", err, ehrUUID.String(), userID)
	}

	ehr.EhrStatus.ID = doc.UID.ObjectID
	ehr.EhrStatus.Type = "EHR_STATUS"

	err = s.SaveEhr(ctx, multiCallTx, procRequest, userID, &ehr)
	if err != nil {
		return nil, fmt.Errorf("SaveEhr error: %w", err)
	}

	txHash, err := multiCallTx.Commit()
	if err != nil {
		return nil, fmt.Errorf("EhrCreateWithID commit error: %w", err)
	}

	for _, txKind := range multiCallTx.GetTxKinds() {
		procRequest.AddEthereumTx(proc.TxKind(txKind), txHash)
	}

	return &ehr, nil
}

func (s *Service) SaveEhr(ctx context.Context, multiCallTx *indexer.MultiCallTx, procRequest *proc.Request, userID string, doc *model.EHR) error {
	ehrUUID, err := uuid.Parse(doc.EhrID.Value)
	if err != nil {
		return fmt.Errorf("ehrUUID parse error: %w ehrID.Value %s", err, doc.EhrID.Value)
	}

	userPubKey, userPrivKey, err := s.Infra.Keystore.Get(userID)
	if err != nil {
		return fmt.Errorf("Keystore.Get error: %w userID %s", err, userID)
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

	procRequest.AddFilecoinTx(proc.TxSaveEhr, CID.String(), dealCID.String(), minerAddr)

	// Index Docs ehr_id -> doc_meta
	{
		ehrIDEncrypted, err := key.EncryptWithAuthData(ehrUUID[:], ehrUUID[:])
		if err != nil {
			return fmt.Errorf("EncryptWithAuthData error: %w ehrID: %s", err, ehrUUID.String())
		}

		CIDEncr, err := keybox.SealAnonymous(CID.Bytes(), userPubKey)
		if err != nil {
			return fmt.Errorf("keybox.SealAnonymous error: %w", err)
		}

		keyEncr, err := keybox.SealAnonymous(key.Bytes(), userPubKey)
		if err != nil {
			return fmt.Errorf("keybox.SealAnonymous error: %w", err)
		}

		docMeta := &model.DocumentMeta{
			Status:    uint8(docStatus.ACTIVE),
			Id:        CID.Bytes(),
			Version:   nil,
			Timestamp: uint32(time.Now().Unix()),
			IsLast:    true,
			Attrs: []ehrIndexer.AttributesAttribute{
				{Code: model.AttributeIDEncr, Value: CIDEncr},
				{Code: model.AttributeKeyEncr, Value: keyEncr},
				{Code: model.AttributeDocUIDHash, Value: make([]byte, 32)},
				{Code: model.AttributeDocUIDEncr, Value: ehrIDEncrypted},
				{Code: model.AttributeDealCid, Value: dealCID.Bytes()},
				{Code: model.AttributeMinerAddress, Value: []byte(minerAddr)},
			},
		}

		packed, err := s.Infra.Index.AddEhrDoc(ctx, types.Ehr, docMeta, userPrivKey, multiCallTx.Nonce())
		if err != nil {
			return fmt.Errorf("Index.AddEhrDoc error: %w", err)
		}

		multiCallTx.Add(uint8(proc.TxAddEhrDoc), packed)
	}

	return nil
}

func (s *Service) GetByID(ctx context.Context, userID, systemID string, ehrUUID *uuid.UUID) ([]byte, error) {
	docMeta, err := s.Infra.Index.GetDocLastByType(ctx, ehrUUID, types.Ehr)
	if err != nil {
		return nil, fmt.Errorf("GetDocLastByType error: %w", err)
	}

	CID, err := cid.Parse(docMeta.Id)
	if err != nil {
		return nil, fmt.Errorf("cid.Parse error: %w", err)
	}

	docUIDEncrypted := docMeta.GetAttr(model.AttributeDocUIDEncr)
	if docUIDEncrypted == nil {
		return nil, errors.ErrFieldIsEmpty("DocUIDEncrypted")
	}

	docDecrypted, err := s.GetDocFromStorageByID(ctx, userID, systemID, &CID, ehrUUID[:], docUIDEncrypted)
	if err != nil && errors.Is(err, errors.ErrIsInProcessing) {
		return nil, err
	} else if err != nil {
		return nil, fmt.Errorf("GetDocFromStorageByID error: %w", err)
	}

	return docDecrypted, nil
}

// GetDocBySubject Get decrypted document by subject
func (s *Service) GetDocBySubject(ctx context.Context, userID, systemID, subjectID, namespace string) (docDecrypted []byte, err error) {
	ehrUUID, err := s.Infra.Index.GetEhrUUIDBySubject(ctx, subjectID, namespace)
	if err != nil {
		return nil, fmt.Errorf("Index.GetEhrUUIDBySubject error: %w. userID: %s subjectID: %s namespace: %s", err, userID, subjectID, namespace)
	}

	// Getting docStorageID
	docMeta, err := s.Infra.Index.GetDocLastByType(ctx, ehrUUID, types.Ehr)
	if err != nil {
		return nil, fmt.Errorf("Index.GetLastDocByType error: %w. ehrUUID: %s", err, ehrUUID.String())
	}

	CID, err := cid.Parse(docMeta.Id)
	if err != nil {
		return nil, fmt.Errorf("cid.Parse error: %w", err)
	}

	docUIDEncrypted := docMeta.GetAttr(model.AttributeDocUIDEncr)
	if docUIDEncrypted == nil {
		return nil, errors.ErrFieldIsEmpty("DocUIDEncrypted")
	}

	// Getting doc from storage
	docDecrypted, err = s.GetDocFromStorageByID(ctx, userID, systemID, &CID, ehrUUID[:], docUIDEncrypted)
	if err != nil && errors.Is(err, errors.ErrIsInProcessing) {
		return nil, err
	} else if err != nil {
		return nil, fmt.Errorf("GetDocFromStorageByID error: %w. userID: %s, doc.CID: %s ehrUUID: %s", err, userID, CID.String(), ehrUUID.String())
	}

	return docDecrypted, nil
}

func (s *Service) CreateSubject(subjectID, subjectNamespace, subType string) (subject base.PartySelf) {
	subject.ExternalRef = &base.ObjectRef{
		ID: base.ObjectID{
			Type:  "HIER_OBJECT_ID", // TODO is it always eq with "HIER_OBJECT_ID"?
			Value: subjectID,
		},
		Namespace: subjectNamespace,
		Type:      subType,
	}

	return
}

func (s *Service) CreateStatus(ehrStatusID string, subject base.PartySelf) (doc *model.EhrStatus, err error) {
	doc = &model.EhrStatus{
		Locatable: base.Locatable{
			Type:            base.EHRStatusItemType,
			ArchetypeNodeID: "openEHR-EHR-EHR_STATUS.generic.v1",
			Name:            base.NewDvText("EHR Status"),
			ObjectVersionID: base.ObjectVersionID{
				// todo FIXIT
				UID: &base.UIDBasedID{
					ObjectID: base.ObjectID{
						Type:  "OBJECT_VERSION_ID",
						Value: ehrStatusID,
					},
				},
			},
		},
		Subject:     subject,
		IsQueryable: true,
		IsModifable: true,
	}

	return doc, nil
}

func (s *Service) UpdateEhr(ctx context.Context, multiCallTx *indexer.MultiCallTx, procRequest *proc.Request, userID, systemID string, ehrUUID *uuid.UUID, status *model.EhrStatus) error {
	docMeta, err := s.Infra.Index.GetDocLastByType(ctx, ehrUUID, types.Ehr)
	if err != nil {
		return fmt.Errorf("Index.GetLastEhrDocByType error: %w. ehrID: %s", err, ehrUUID.String())
	}

	CID, err := cid.Parse(docMeta.Id)
	if err != nil {
		return fmt.Errorf("cid.Parse error: %w", err)
	}

	docUIDEncrypted := docMeta.GetAttr(model.AttributeDocUIDEncr)
	if docUIDEncrypted == nil {
		return errors.ErrFieldIsEmpty("DocUIDEncrypted")
	}

	ehrDecrypted, err := s.GetDocFromStorageByID(ctx, userID, systemID, &CID, ehrUUID[:], docUIDEncrypted)
	if err != nil && errors.Is(err, errors.ErrIsInProcessing) {
		return err
	} else if err != nil {
		return fmt.Errorf("GetDocFromStorageByID error: %w. userID: %s StorageID: %x ehrID: %s", err, userID, &CID, ehrUUID.String())
	}

	var ehr model.EHR
	if err = json.Unmarshal(ehrDecrypted, &ehr); err != nil {
		return fmt.Errorf("ehr unmarshal error: %w", err)
	}

	if status.UID.Value != ehr.EhrStatus.ID.Value {
		ehr.EhrStatus.ID.Value = status.UID.Value
		if err = s.SaveEhr(ctx, multiCallTx, procRequest, userID, &ehr); err != nil {
			return fmt.Errorf("ehr save error: %w", err)
		}
	}

	return nil
}

func (s *Service) SaveStatus(ctx context.Context, multiCallTx *indexer.MultiCallTx, procRequest *proc.Request, userID, systemID string, ehrUUID *uuid.UUID, status *model.EhrStatus) error {
	userPubKey, userPrivKey, err := s.Infra.Keystore.Get(userID)
	if err != nil {
		return fmt.Errorf("Keystore.Get error: %w userID %s", err, userID)
	}

	// Document encryption key generation
	key := chachaPoly.GenerateKey()

	objectVersionID, err := base.NewObjectVersionID(status.UID.Value, systemID)
	if err != nil {
		return fmt.Errorf("SaveStatus error: %w versionUID %s ehrSystemID %s", err, objectVersionID.String(), systemID)
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

	procRequest.AddFilecoinTx(proc.TxSaveEhrStatus, CID.String(), dealCID.String(), minerAddr)

	// Index subject and namespace
	{
		subjectID := status.Subject.ExternalRef.ID.Value
		subjectNamespace := status.Subject.ExternalRef.Namespace

		setSubjectPacked, err := s.Infra.Index.SetEhrSubject(ctx, ehrUUID, subjectID, subjectNamespace, userPrivKey, multiCallTx.Nonce())
		if err != nil {
			return fmt.Errorf("Index.SetSubject error: %w ehrID: %s subjectID: %s subjectNamespace: %s", err, ehrUUID.String(), subjectID, subjectNamespace)
		}

		multiCallTx.Add(uint8(proc.TxSetEhrBySubject), setSubjectPacked)
	}

	// Index Docs ehr_id -> doc_meta
	{
		statusIDEncrypted, err := key.EncryptWithAuthData([]byte(objectVersionID.String()), ehrUUID[:])
		if err != nil {
			return fmt.Errorf("EncryptWithAuthData error: %w ehrID: %s statusUid: %s", err, ehrUUID.String(), status.UID.Value)
		}

		CIDEncr, err := keybox.SealAnonymous(CID.Bytes(), userPubKey)
		if err != nil {
			return fmt.Errorf("keybox.SealAnonymous error: %w", err)
		}

		keyEncr, err := keybox.SealAnonymous(key.Bytes(), userPubKey)
		if err != nil {
			return fmt.Errorf("keybox.SealAnonymous error: %w", err)
		}

		docMeta := &model.DocumentMeta{
			Status:    uint8(docStatus.ACTIVE),
			Id:        CID.Bytes(),
			Version:   objectVersionID.VersionBytes()[:],
			Timestamp: uint32(time.Now().Unix()),
			IsLast:    true,
			Attrs: []ehrIndexer.AttributesAttribute{
				{Code: model.AttributeIDEncr, Value: CIDEncr},
				{Code: model.AttributeKeyEncr, Value: keyEncr},
				{Code: model.AttributeDocUIDHash, Value: baseDocumentUIDHash[:]},
				{Code: model.AttributeDocUIDEncr, Value: statusIDEncrypted},
				{Code: model.AttributeDealCid, Value: dealCID.Bytes()},
				{Code: model.AttributeMinerAddress, Value: []byte(minerAddr)},
			},
		}

		packed, err := s.Infra.Index.AddEhrDoc(ctx, types.EhrStatus, docMeta, userPrivKey, multiCallTx.Nonce())
		if err != nil {
			return fmt.Errorf("Index.AddEhrDoc error: %w", err)
		}

		multiCallTx.Add(uint8(proc.TxAddEhrDoc), packed)
	}

	return nil
}

func (s *Service) UpdateStatus(ctx context.Context, procRequest *proc.Request, userID, systemID string, ehrUUID *uuid.UUID, status *model.EhrStatus) error {
	_, userPrivKey, err := s.Infra.Keystore.Get(userID)
	if err != nil {
		return fmt.Errorf("Keystore.Get error: %w userID %s", err, userID)
	}

	multiCallTx, err := s.Infra.Index.MultiCallEhrNew(ctx, userPrivKey)
	if err != nil {
		return fmt.Errorf("MultiCallEhrNew error: %w", err)
	}

	if err := s.SaveStatus(ctx, multiCallTx, procRequest, userID, systemID, ehrUUID, status); err != nil {
		return fmt.Errorf("SaveStatus error: %w", err)
	}

	// TODO i dont like this logic, because in method GetByID we always grab whole data from filecoin, which contain last status id. It need fix it.
	if err := s.UpdateEhr(ctx, multiCallTx, procRequest, userID, systemID, ehrUUID, status); err != nil {
		return fmt.Errorf("UpdateEhr error: %w", err)
	}

	txHash, err := multiCallTx.Commit()
	if err != nil {
		return fmt.Errorf("UpdateStatus commit error: %w", err)
	}

	for _, txKind := range multiCallTx.GetTxKinds() {
		procRequest.AddEthereumTx(proc.TxKind(txKind), txHash)
	}

	return nil
}

// GetStatus Get current (last) status of EHR document
func (s *Service) GetStatus(ctx context.Context, userID, systemID string, ehrUUID *uuid.UUID) (*model.EhrStatus, error) {
	docMeta, err := s.Infra.Index.GetDocLastByType(ctx, ehrUUID, types.EhrStatus)
	if err != nil {
		return nil, fmt.Errorf("Index.GetLastEhrDocByType error: %w. ehrID: %s", err, ehrUUID.String())
	}

	CID, err := cid.Parse(docMeta.Id)
	if err != nil {
		return nil, fmt.Errorf("cid.Parse error: %w", err)
	}

	docUIDEncrypted := docMeta.GetAttr(model.AttributeDocUIDEncr)
	if docUIDEncrypted == nil {
		return nil, errors.ErrFieldIsEmpty("DocUIDEncrypted")
	}

	docDecrypted, err := s.GetDocFromStorageByID(ctx, userID, systemID, &CID, ehrUUID[:], docUIDEncrypted)
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

func (s *Service) GetStatusByVersionID(ctx context.Context, userID, systemID string, ehrUUID *uuid.UUID, versionID *base.ObjectVersionID) ([]byte, error) {
	baseDocumentUID := versionID.BasedID()
	baseDocumentUIDHash := sha3.Sum256([]byte(baseDocumentUID))

	docMeta, err := s.Infra.Index.GetDocByVersion(ctx, ehrUUID, types.EhrStatus, &baseDocumentUIDHash, versionID.VersionBytes())
	if err != nil {
		if errors.Is(err, errors.ErrNotFound) {
			return nil, err
		}

		return nil, fmt.Errorf("Index.GetDocByVersion error: %w", err)
	}

	CID, err := cid.Parse(docMeta.Id)
	if err != nil {
		return nil, fmt.Errorf("cid.Parse error: %w", err)
	}

	docUIDEncrypted := docMeta.GetAttr(model.AttributeDocUIDEncr)
	if docUIDEncrypted == nil {
		return nil, errors.ErrFieldIsEmpty("DocUIDEncrypted")
	}

	docDecrypted, err := s.GetDocFromStorageByID(ctx, userID, systemID, &CID, ehrUUID[:], docUIDEncrypted)
	if err != nil {
		if errors.Is(err, errors.ErrIsInProcessing) {
			return nil, err
		} else if errors.Is(err, errors.ErrNotFound) {
			return nil, err
		}

		return nil, fmt.Errorf("GetDocFromStorageByID error: %w", err)
	}

	return docDecrypted, nil
}

func (s *Service) GetStatusByNearestTime(ctx context.Context, userID, systemID string, ehrUUID *uuid.UUID, nearestTime time.Time) ([]byte, error) {
	docMeta, err := s.Infra.Index.GetDocByTime(ctx, ehrUUID, types.EhrStatus, uint32(nearestTime.Unix()))
	if err != nil && errors.Is(err, errors.ErrNotFound) {
		return nil, err
	} else if err != nil {
		return nil, fmt.Errorf("Index.GetDocByTime error: %w ehrID %s nearestTime %s docType %s", err, ehrUUID.String(), nearestTime.String(), types.EhrStatus)
	}

	CID, err := cid.Parse(docMeta.Id)
	if err != nil {
		return nil, fmt.Errorf("cid.Parse error: %w", err)
	}

	docUIDEncrypted := docMeta.GetAttr(model.AttributeDocUIDEncr)
	if docUIDEncrypted == nil {
		return nil, errors.ErrFieldIsEmpty("DocUIDEncrypted")
	}

	docDecrypted, err := s.GetDocFromStorageByID(ctx, userID, systemID, &CID, ehrUUID[:], docUIDEncrypted)
	if err != nil && errors.Is(err, errors.ErrIsInProcessing) {
		return nil, err
	} else if err != nil {
		return nil, fmt.Errorf("GetDocFromStorageByID error: %w", err)
	}

	return docDecrypted, nil
}

func (s *Service) ValidateEhr(ehr *model.EHR) bool {
	// TODO
	return true
}

func (s *Service) ValidateStatus(status *model.EhrStatus) bool {
	// TODO
	return true
}
