package indexer

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/google/uuid"
	"golang.org/x/crypto/sha3"

	"hms/gateway/pkg/access"
	"hms/gateway/pkg/docs/model"
	"hms/gateway/pkg/errors"
	"hms/gateway/pkg/indexer/users"
	userModel "hms/gateway/pkg/user/model"
)

func (i *Index) UserGroupCreate(ctx context.Context, groupID *uuid.UUID, idEncr, keyEncr, contentEncr []byte, privKey *[32]byte, nonce *big.Int) ([]byte, error) {
	userKey, err := crypto.ToECDSA(privKey[:])
	if err != nil {
		return nil, fmt.Errorf("crypto.ToECDSA error: %w", err)
	}

	userAddress := crypto.PubkeyToAddress(userKey.PublicKey)

	if nonce == nil {
		nonce, err = i.usersNonce(ctx, &userAddress)
		if err != nil {
			return nil, fmt.Errorf("userNonce error: %w address: %s", err, userAddress.String())
		}
	}

	attrs := []users.AttributesAttribute{
		{Code: model.AttributeKeyEncr, Value: keyEncr},         // encrypted by userKey
		{Code: model.AttributeIDEncr, Value: idEncr},           // encrypted by group key
		{Code: model.AttributeContentEncr, Value: contentEncr}, // encrypted by group key
	}

	data, err := i.usersAbi.Pack("userGroupCreate", sha3.Sum256(groupID[:]), attrs, userAddress, make([]byte, signatureLength))
	if err != nil {
		return nil, fmt.Errorf("abi.Pack1 error: %w", err)
	}

	signature, err := makeSignature(data, nonce, userKey)
	if err != nil {
		return nil, fmt.Errorf("makeSignature error: %w", err)
	}

	data, err = i.usersAbi.Pack("userGroupCreate", sha3.Sum256(groupID[:]), attrs, userAddress, signature)
	if err != nil {
		return nil, fmt.Errorf("abi.Pack2 error: %w", err)
	}

	return data, nil
}

func (i *Index) UserGroupGetByID(ctx context.Context, groupID *uuid.UUID) (*userModel.UserGroup, error) {
	groupIDHash := sha3.Sum256(groupID[:])

	ug, err := i.users.UserGroupGetByID(&bind.CallOpts{Context: ctx}, groupIDHash)
	if err != nil {
		return nil, fmt.Errorf("ehrIndex.UserGroupGetByID error: %w", err)
	}

	if len(ug.Attrs) == 0 {
		return nil, errors.ErrNotFound
	}

	contentEncr := model.AttributesUsers(ug.Attrs).GetByCode(model.AttributeContentEncr)
	if contentEncr == nil {
		return nil, errors.ErrFieldIsEmpty("ContentEncr")
	}

	groupKeyEncr := model.AttributesUsers(ug.Attrs).GetByCode(model.AttributeKeyEncr)
	if groupKeyEncr == nil {
		return nil, errors.ErrFieldIsEmpty("KeyEncr")
	}

	userGroup := &userModel.UserGroup{
		GroupID:      groupID,
		ContentEncr:  contentEncr,
		GroupKeyEncr: groupKeyEncr,
		Members:      []string{},
	}

	for _, m := range ug.Members {
		userGroup.MembersEncr = append(userGroup.MembersEncr, m.UserIDEncr)
	}

	return userGroup, nil
}

func (i *Index) UserGroupAddUser(ctx context.Context, addingUserID string, level access.Level, groupID *uuid.UUID, addingUserIDEncr, groupKeyEncr []byte, privKey *[32]byte, nonce *big.Int) (string, error) {
	var uID [32]byte

	copy(uID[:], addingUserID)

	userKey, err := crypto.ToECDSA(privKey[:])
	if err != nil {
		return "", fmt.Errorf("crypto.ToECDSA error: %w", err)
	}

	userAddress := crypto.PubkeyToAddress(userKey.PublicKey)

	if nonce == nil {
		nonce, err = i.usersNonce(ctx, &userAddress)
		if err != nil {
			return "", fmt.Errorf("userNonce error: %w address: %s", err, userAddress.String())
		}
	}

	params := users.IUsersGroupAddUserParams{
		GroupIDHash: sha3.Sum256(groupID[:]),
		UserIDHash:  sha3.Sum256(uID[:]),
		Level:       level,
		UserIDEncr:  addingUserIDEncr,
		KeyEncr:     groupKeyEncr,
		Signer:      userAddress,
		Signature:   make([]byte, signatureLength),
	}

	data, err := i.usersAbi.Pack("groupAddUser", params)
	if err != nil {
		return "", fmt.Errorf("abi.Pack error: %w", err)
	}

	params.Signature, err = makeSignature(data, nonce, userKey)
	if err != nil {
		return "", fmt.Errorf("makeSignature error: %w", err)
	}

	tx, err := i.users.GroupAddUser(i.transactOpts, params)
	if err != nil {
		if strings.Contains(err.Error(), "DNY") {
			return "", errors.ErrAccessDenied
		} else if strings.Contains(err.Error(), "AEX") {
			return "", errors.ErrAlreadyExist
		}

		return "", fmt.Errorf("ehrIndex.UserGroupCreate error: %w", err)
	}

	return tx.Hash().Hex(), nil
}

func (i *Index) UserGroupRemoveUser(ctx context.Context, removingUserID string, groupID *uuid.UUID, privKey *[32]byte, nonce *big.Int) (string, error) {
	var uID [32]byte

	copy(uID[:], removingUserID)

	userKey, err := crypto.ToECDSA(privKey[:])
	if err != nil {
		return "", fmt.Errorf("crypto.ToECDSA error: %w", err)
	}

	userAddress := crypto.PubkeyToAddress(userKey.PublicKey)

	if nonce == nil {
		nonce, err = i.usersNonce(ctx, &userAddress)
		if err != nil {
			return "", fmt.Errorf("userNonce error: %w address: %s", err, userAddress.String())
		}
	}

	groupIDHash := sha3.Sum256(groupID[:])
	removingUserIDHash := sha3.Sum256(uID[:])

	data, err := i.usersAbi.Pack("groupRemoveUser", groupIDHash, removingUserIDHash, userAddress, make([]byte, signatureLength))
	if err != nil {
		return "", fmt.Errorf("abi.Pack error: %w", err)
	}

	signature, err := makeSignature(data, nonce, userKey)
	if err != nil {
		return "", fmt.Errorf("makeSignature error: %w", err)
	}

	tx, err := i.users.GroupRemoveUser(i.transactOpts, groupIDHash, removingUserIDHash, userAddress, signature)
	if err != nil {
		if strings.Contains(err.Error(), "DNY") {
			return "", errors.ErrAccessDenied
		} else if strings.Contains(err.Error(), "NFD") {
			return "", errors.ErrNotFound
		}

		return "", fmt.Errorf("users.GroupRemoveUser error: %w", err)
	}

	return tx.Hash().Hex(), nil
}
