package composition

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"time"

	"golang.org/x/crypto/sha3"

	"github.com/google/uuid"
	"github.com/ipfs/go-cid"

	"hms/gateway/pkg/crypto/chachaPoly"
	"hms/gateway/pkg/crypto/keybox"
	"hms/gateway/pkg/docs/model"
	"hms/gateway/pkg/docs/model/base"
	proc "hms/gateway/pkg/docs/service/processing"
	"hms/gateway/pkg/docs/status"
	"hms/gateway/pkg/docs/types"
	"hms/gateway/pkg/errors"
	"hms/gateway/pkg/indexer"
	"hms/gateway/pkg/indexer/ehrIndexer"
)

type GroupAccessService interface {
	Default() *model.GroupAccess
}

type Indexer interface {
	MultiCallEhrNew(ctx context.Context, pk *[32]byte) (*indexer.MultiCallTx, error)
	GetDocByVersion(ctx context.Context, ehrUUID *uuid.UUID, docType types.DocumentType, docBaseUIDHash, version *[32]byte) (*model.DocumentMeta, error)
	AddEhrDoc(ctx context.Context, docType types.DocumentType, docMeta *model.DocumentMeta, privKey *[32]byte, nonce *big.Int) ([]byte, error)
	GetDocLastByBaseID(ctx context.Context, ehrUUID *uuid.UUID, docType types.DocumentType, docBaseUIDHash *[32]byte) (*model.DocumentMeta, error)
	DeleteDoc(ctx context.Context, ehrUUID *uuid.UUID, docType types.DocumentType, docBaseUIDHash, version, privKey *[32]byte, nonce *big.Int) (string, error)
}

type IpfsService interface {
	Add(ctx context.Context, fileContent []byte) (*cid.Cid, error)
}

type FileCoinService interface {
	StartDeal(ctx context.Context, CID *cid.Cid, dataSizeBytes uint64) (*cid.Cid, string, error)
}

type DocumentsSvc interface {
	GetDocFromStorageByID(ctx context.Context, userID, systemID string, CID *cid.Cid, authData, docIDEncrypted []byte) ([]byte, error)
}

type KeyStore interface {
	Get(userID string) (publicKey, privateKey *[32]byte, err error)
}

type Compressor interface {
	Compress(decompressedData []byte) (compressedData []byte, err error)
}

type Service struct {
	indexer            Indexer
	ipfs               IpfsService
	fileCoin           FileCoinService
	keyStore           KeyStore
	compressor         Compressor
	docSvc             DocumentsSvc
	groupAccessService GroupAccessService
}

func NewCompositionService(
	indexer Indexer,
	ipfs IpfsService,
	fileCoin FileCoinService,
	keyStore KeyStore,
	compressor Compressor,
	docSvc DocumentsSvc,
	groupAccessService GroupAccessService,
) *Service {
	return &Service{
		docSvc:             docSvc,
		indexer:            indexer,
		ipfs:               ipfs,
		fileCoin:           fileCoin,
		keyStore:           keyStore,
		compressor:         compressor,
		groupAccessService: groupAccessService,
	}
}

func (s *Service) Create(ctx context.Context, userID, systemID string, ehrUUID, groupAccessUUID *uuid.UUID, composition *model.Composition, procRequest *proc.Request) (*model.Composition, error) {
	var (
		groupAccess = s.groupAccessService.Default()
		err         error
	)

	_, userPrivKey, err := s.keyStore.Get(userID)
	if err != nil {
		return nil, fmt.Errorf("Keystore.Get error: %w userID %s", err, userID)
	}

	multiCallTx, err := s.indexer.MultiCallEhrNew(ctx, userPrivKey)
	if err != nil {
		return nil, fmt.Errorf("MultiCallEhrNew error: %w userID %s", err, userID)
	}

	/*
		if groupAccessUUID != nil {
			groupAccess, err = s.groupAccessService.Get(ctx, userID, groupAccessUUID)
			if err != nil {
				return nil, fmt.Errorf("groupAccessService.Get error: %w userID %s groupAccessUUID %s", err, userID, groupAccessUUID.String())
			}
		}
	*/

	err = s.save(ctx, multiCallTx, procRequest, userID, systemID, ehrUUID, groupAccess, composition)
	if err != nil {
		return nil, fmt.Errorf("Composition %s save error: %w", composition.UID.Value, err)
	}

	txHash, err := multiCallTx.Commit()
	if err != nil {
		return nil, fmt.Errorf("Create composition commit error: %w", err)
	}

	for _, txKind := range multiCallTx.GetTxKinds() {
		procRequest.AddEthereumTx(proc.TxKind(txKind), txHash)
	}

	return composition, nil
}

func (s *Service) Update(ctx context.Context, procRequest *proc.Request, userID, systemID string, ehrUUID, groupAccessUUID *uuid.UUID, composition *model.Composition) (*model.Composition, error) {
	var (
		groupAccess = s.groupAccessService.Default()
		err         error
	)

	_, userPrivKey, err := s.keyStore.Get(userID)
	if err != nil {
		return nil, fmt.Errorf("Keystore.Get error: %w userID %s", err, userID)
	}

	multiCallTx, err := s.indexer.MultiCallEhrNew(ctx, userPrivKey)
	if err != nil {
		return nil, fmt.Errorf("MultiCallEhrNew error: %w userID %s", err, userID)
	}

	/*
		if groupAccessUUID != nil {
			groupAccess, err = s.groupAccessService.Get(ctx, userID, groupAccessUUID)
			if err != nil {
				return nil, fmt.Errorf("groupAccessService.Get error: %w userID %s groupAccessUUID %s", err, userID, groupAccessUUID.String())
			}
		}
	*/

	if err = s.increaseVersion(composition, systemID); err != nil {
		return nil, fmt.Errorf("Composition increaseVersion error: %w composition.UID %s", err, composition.UID.Value)
	}

	err = s.save(ctx, multiCallTx, procRequest, userID, systemID, ehrUUID, groupAccess, composition)
	if err != nil {
		return nil, fmt.Errorf("Composition save error: %w userID %s ehrUUID %s composition.UID %s", err, userID, ehrUUID.String(), composition.UID.Value)
	}

	txHash, err := multiCallTx.Commit()
	if err != nil {
		return nil, fmt.Errorf("Update composition commit error: %w", err)
	}

	for _, txKind := range multiCallTx.GetTxKinds() {
		procRequest.AddEthereumTx(proc.TxKind(txKind), txHash)
	}

	// TODO what we should do with prev composition?
	return composition, nil
}

func (s *Service) increaseVersion(c *model.Composition, ehrSystemID string) error {
	if c == nil || c.UID == nil || c.UID.Value == "" {
		return fmt.Errorf("%w Incorrect composition UID", errors.ErrIncorrectFormat)
	}

	objectVersionID, err := base.NewObjectVersionID(c.UID.Value, ehrSystemID)
	if err != nil {
		return fmt.Errorf("increaseVersion error: %w versionUID %s ehrSystemID %s", err, objectVersionID.String(), ehrSystemID)
	}

	if _, err := objectVersionID.IncreaseUIDVersion(); err != nil {
		return fmt.Errorf("Composition %s IncreaseUIDVersion error: %w", c.UID.Value, err)
	}

	c.UID.Value = objectVersionID.String()

	return nil
}

func (s *Service) save(ctx context.Context, multiCallTx *indexer.MultiCallTx, procRequest *proc.Request, userID, systemID string, ehrUUID *uuid.UUID, groupAccess *model.GroupAccess, doc *model.Composition) error {
	userPubKey, userPrivKey, err := s.keyStore.Get(userID)
	if err != nil {
		return fmt.Errorf("Keystore.Get error: %w userID %s", err, userID)
	}

	objectVersionID, err := base.NewObjectVersionID(doc.UID.Value, systemID)
	if err != nil {
		return fmt.Errorf("saving error: %w versionUID %s ehrSystemID %s", err, objectVersionID, systemID)
	}

	baseDocumentUID := []byte(objectVersionID.BasedID())
	baseDocumentUIDHash := sha3.Sum256(baseDocumentUID)

	// Checking the existence of the Composition
	docMeta, err := s.indexer.GetDocByVersion(ctx, ehrUUID, types.Composition, &baseDocumentUIDHash, objectVersionID.VersionBytes())
	if err != nil && !errors.Is(err, errors.ErrNotFound) {
		return fmt.Errorf("Index.GetDocByVersion error: %w", err)
	} else if docMeta != nil {
		return fmt.Errorf("%w objectVersionID %s", errors.ErrAlreadyExist, objectVersionID.String())
	}

	docBytes, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("Composition marshal error: %w", err)
	}

	if s.compressor != nil {
		docBytes, err = s.compressor.Compress(docBytes)
		if err != nil {
			return fmt.Errorf("Compress error: %w", err)
		}
	}

	// Document encryption key generation
	key := chachaPoly.GenerateKey()

	// Document encryption
	docEncrypted, err := key.EncryptWithAuthData(docBytes, []byte(objectVersionID.String()))
	if err != nil {
		return fmt.Errorf("EncryptWithAuthData error: %w", err)
	}

	// IPFS saving
	CID, err := s.ipfs.Add(ctx, docEncrypted)
	if err != nil {
		return fmt.Errorf("IpfsClient.Add error: %w", err)
	}

	// Filecoin saving
	dealCID, minerAddr, err := s.fileCoin.StartDeal(ctx, CID, uint64(len(docEncrypted)))
	if err != nil {
		return fmt.Errorf("FilecoinClient.StartDeal error: %w", err)
	}
	//dealCID := fakeData.Cid()
	//minerAddr := "123"

	docIDEncrypted, err := key.EncryptWithAuthData([]byte(objectVersionID.String()), ehrUUID[:])
	if err != nil {
		return fmt.Errorf("EncryptWithAuthData error: %w", err)
	}

	// Add filecoin tx
	procRequest.AddFilecoinTx(proc.TxSaveComposition, CID.String(), dealCID.String(), minerAddr)

	// Index Docs ehr_id -> doc_meta
	{
		keyEncr, err := keybox.SealAnonymous(key.Bytes(), userPubKey)
		if err != nil {
			return fmt.Errorf("keybox.SealAnonymous error: %w", err)
		}

		CIDEncr, err := keybox.SealAnonymous(CID.Bytes(), userPubKey)
		if err != nil {
			return fmt.Errorf("keybox.SealAnonymous error: %w", err)
		}

		docMeta := &model.DocumentMeta{
			Status:    uint8(status.ACTIVE),
			Id:        CID.Bytes(),
			Version:   objectVersionID.VersionBytes()[:],
			Timestamp: uint32(time.Now().Unix()),
			IsLast:    true,
			Attrs: []ehrIndexer.AttributesAttribute{
				{Code: model.AttributeIDEncr, Value: CIDEncr},
				{Code: model.AttributeKeyEncr, Value: keyEncr},
				{Code: model.AttributeDocBaseUIDHash, Value: baseDocumentUIDHash[:]},
				{Code: model.AttributeDocUIDEncr, Value: docIDEncrypted},
				{Code: model.AttributeDealCid, Value: dealCID.Bytes()},
				{Code: model.AttributeMinerAddress, Value: []byte(minerAddr)},
			},
		}

		packed, err := s.indexer.AddEhrDoc(ctx, types.Composition, docMeta, userPrivKey, multiCallTx.Nonce())
		if err != nil {
			return fmt.Errorf("Index.AddEhrDoc error: %w", err)
		}

		multiCallTx.Add(uint8(proc.TxAddEhrDoc), packed)
	}

	// Index DataSearch
	_ = groupAccess
	/* TODO
	docStorageIDEncrypted, err := groupAccess.Key.EncryptWithAuthData(cidBytes[:], groupAccess.GroupUUID[:])
	if err != nil {
		return fmt.Errorf("EncryptWithAuthData error: %w", err)
	}

	if err = s.DataSearchIndex.UpdateIndexWithNewContent(doc.Content, groupAccess, docStorageIDEncrypted); err != nil {
		return fmt.Errorf("UpdateIndexWithNewContent error: %w", err)
	}
	*/

	// Index Access
	/*
		{
			accessID := sha3.Sum256(append(CID.Bytes()[:], []byte(userID)...))

			packed, err := s.Infra.Index.SetDocAccess(ctx, &accessID, CID.Bytes(), keyEncrypted, uint8(access.Owner), userPrivKey, multiCallTx.Nonce())
			if err != nil {
				return fmt.Errorf("Index.SetDocAccess error: %w", err)
			}

			multiCallTx.Add(uint8(proc.TxSetDocKeyEncrypted), packed)
		}
	*/

	return nil
}

func (s *Service) GetLastByBaseID(ctx context.Context, userID, systemID string, ehrUUID *uuid.UUID, versionUID string) (*model.Composition, error) {
	objectVersionID, err := base.NewObjectVersionID(versionUID, systemID)
	if err != nil {
		return nil, fmt.Errorf("GetLastByBaseID error: %w versionUID %s ehrSystemID %s", err, objectVersionID.String(), systemID)
	}

	baseDocumentUID := []byte(objectVersionID.BasedID())
	baseDocumentUIDHash := sha3.Sum256(baseDocumentUID)

	docMeta, err := s.indexer.GetDocLastByBaseID(ctx, ehrUUID, types.Composition, &baseDocumentUIDHash)
	if err != nil {
		return nil, fmt.Errorf("GetLastVersionDocIndexByBaseID error: %w userID %s objectVersionID %s", err, userID, objectVersionID)
	}

	if docMeta.Status == uint8(status.DELETED) {
		return nil, fmt.Errorf("GetLastByBaseID error: %w", errors.ErrAlreadyDeleted)
	}

	CID, err := cid.Parse(docMeta.Id)
	if err != nil {
		return nil, fmt.Errorf("cid.Parse error: %w", err)
	}

	docUIDEncrypted := docMeta.GetAttr(model.AttributeDocUIDEncr)
	if docUIDEncrypted == nil {
		return nil, errors.ErrFieldIsEmpty("DocUIDEncrypted")
	}

	docDecrypted, err := s.docSvc.GetDocFromStorageByID(ctx, userID, systemID, &CID, ehrUUID[:], docUIDEncrypted)
	if err != nil && errors.Is(err, errors.ErrIsInProcessing) {
		return nil, err
	} else if err != nil {
		return nil, fmt.Errorf("GetDocFromStorageByID error: %w userID %s storageID %s", err, userID, &CID)
	}

	var composition *model.Composition
	if err = json.Unmarshal(docDecrypted, &composition); err != nil {
		return nil, fmt.Errorf("Composition unmarshal error: %w", err)
	}

	return composition, nil
}

func (s *Service) GetByID(ctx context.Context, userID, systemID string, ehrUUID *uuid.UUID, versionUID string) (*model.Composition, error) {
	objectVersionID, err := base.NewObjectVersionID(versionUID, systemID)
	if err != nil {
		return nil, fmt.Errorf("NewObjectVersionID error: %w versionUID %s ehrSystemID %s", err, versionUID, systemID)
	}

	baseDocumentUID := []byte(objectVersionID.BasedID())
	baseDocumentUIDHash := sha3.Sum256(baseDocumentUID)

	docMeta, err := s.indexer.GetDocByVersion(ctx, ehrUUID, types.Composition, &baseDocumentUIDHash, objectVersionID.VersionBytes())
	if err != nil && errors.Is(err, errors.ErrNotFound) {
		return nil, errors.ErrNotFound
	} else if err != nil {
		return nil, fmt.Errorf("Index.GetDocByVersion error: %w ehrUUID %s objectVersionID %s", err, ehrUUID.String(), objectVersionID.String())
	}

	if docMeta.Status == uint8(status.DELETED) {
		return nil, fmt.Errorf("GetCompositionByID error: %w", errors.ErrAlreadyDeleted)
	}

	CID, err := cid.Parse(docMeta.Id)
	if err != nil {
		return nil, fmt.Errorf("cid.Parse error: %w", err)
	}

	docUIDEncrypted := docMeta.GetAttr(model.AttributeDocUIDEncr)
	if docUIDEncrypted == nil {
		return nil, errors.ErrFieldIsEmpty("DocUIDEncrypted")
	}

	docDecrypted, err := s.docSvc.GetDocFromStorageByID(ctx, userID, systemID, &CID, ehrUUID[:], docUIDEncrypted)
	if err != nil && errors.Is(err, errors.ErrIsInProcessing) {
		return nil, err
	} else if err != nil {
		return nil, fmt.Errorf("GetDocFromStorageByID error: %w userID %s CID %x", err, userID, CID.String())
	}

	var composition model.Composition
	if err = json.Unmarshal(docDecrypted, &composition); err != nil {
		return nil, fmt.Errorf("Composition unmarshal error: %w", err)
	}

	return &composition, nil
}

func (s *Service) DeleteByID(ctx context.Context, procRequest *proc.Request, ehrUUID *uuid.UUID, versionUID, userID, systemID string) (string, error) {
	objectVersionID, err := base.NewObjectVersionID(versionUID, systemID)
	if err != nil {
		return "", fmt.Errorf("NewObjectVersionID error: %w versionUID %s ehrSystemID %s", err, versionUID, systemID)
	}

	_, userPrivKey, err := s.keyStore.Get(userID)
	if err != nil {
		return "", fmt.Errorf("Keystore.Get error: %w userID %s", err, userID)
	}

	baseDocumentUID := []byte(objectVersionID.BasedID())
	baseDocumentUIDHash := sha3.Sum256(baseDocumentUID)

	txHash, err := s.indexer.DeleteDoc(ctx, ehrUUID, types.Composition, &baseDocumentUIDHash, objectVersionID.VersionBytes(), userPrivKey, nil)
	if err != nil {
		if errors.Is(err, errors.ErrNotFound) {
			return "", err
		}
		return "", fmt.Errorf("Index.DeleteDoc error: %w", err)
	}

	procRequest.AddEthereumTx(proc.TxDeleteDoc, txHash)

	// Waiting for tx processed and pending nonce increased
	//time.Sleep(common.BlockchainTxProcAwaitTime)

	if _, err = objectVersionID.IncreaseUIDVersion(); err != nil {
		return "", fmt.Errorf("IncreaseUIDVersion error: %w objectVersionID %s", err, objectVersionID.String())
	}

	return objectVersionID.String(), nil
}

func (s *Service) DefaultGroupAccess() *model.GroupAccess {
	return s.groupAccessService.Default()
}
