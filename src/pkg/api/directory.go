package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/bsn-si/IPEHR-gateway/src/pkg/common"
	"github.com/bsn-si/IPEHR-gateway/src/pkg/docs/model"
	"github.com/bsn-si/IPEHR-gateway/src/pkg/docs/service/processing"
	"github.com/bsn-si/IPEHR-gateway/src/pkg/errors"
	"github.com/bsn-si/IPEHR-gateway/src/pkg/helper"
	userModel "github.com/bsn-si/IPEHR-gateway/src/pkg/user/model"
)

type DirectoryService interface {
	NewProcRequest(reqID, userID, ehrUUID string, kind processing.RequestKind) (processing.RequestInterface, error)
	Create(ctx context.Context, req processing.RequestInterface, systemID string, ehrUUID *uuid.UUID, user *userModel.UserInfo, d *model.Directory) error
	Update(ctx context.Context, req processing.RequestInterface, systemID string, ehrUUID *uuid.UUID, user *userModel.UserInfo, d *model.Directory) error
	Delete(ctx context.Context, req processing.RequestInterface, systemID string, ehrUUID *uuid.UUID, versionUID, userID string) (string, error)
	GetByTime(ctx context.Context, systemID string, ehrUUID *uuid.UUID, userID string, versionTime time.Time) (*model.Directory, error)
	GetByID(ctx context.Context, userID string, versionUID string) (*model.Directory, error)
}

type DirectoryHandler struct {
	service     DirectoryService
	userService UserService
	indexer     Indexer
	baseURL     string
}

func NewDirectoryHandler(cS DirectoryService, uS UserService, indexer Indexer, baseURL string) *DirectoryHandler {
	return &DirectoryHandler{
		service:     cS,
		userService: uS,
		indexer:     indexer,
		baseURL:     baseURL,
	}
}

// Create
// @Summary      Create DIRECTORY
// @Description  https://specifications.openehr.org/releases/ITS-REST/latest/ehr.html#tag/DIRECTORY/operation/directory_create
// @Tags         DIRECTORY
// @Param        Authorization  header    string  true   "Bearer AccessToken"
// @Param        AuthUserId     header    string  true   "UserId UUID"
// @Param        Prefer         header    string     true  "Request header to indicate the preference over response details. The response will contain the entire resource when the Prefer header has a value of return=representation."  Enums: ("return=representation", "return=minimal") default("return=minimal")
// @Param        ehr_id         path      string  true  "EHR identifier taken from EHR.ehr_id.value. Example: 7d44b88c-4199-4bad-97dc-d78268e01398"
// @Header       201            {string}  Etag   "The ETag (i.e. entity tag) response header is the version_uid identifier, enclosed by double quotes. Example: \"8849182c-82ad-4088-a07f-48ead4180515::openEHRSys.example.com::1\""
// @Header       201            {string}  Location   "{baseUrl}/ehr/{ehr_id}/directory/{version_uid}"
// @Header       201            {string}  RequestID  "Request identifier"
// @Accept       json
// @Produce      json
// @Success      201  {object}  model.Directory  "Is returned when the DIRECTORY was successfully created."
// @Failure      400  "Is returned when the request has invalid content"
// @Failure      404  "Is returned when an EHR with {ehr_id}  does not exist"
// @Failure      409  "Is returned when a resource with same identifier(s) already exists"
// @Failure      500  "Is returned when an unexpected error occurs while processing a request"
// @Router       /ehr/{ehr_id}/directory [post]
func (h *DirectoryHandler) Create(ctx *gin.Context) {
	errResponse := model.ErrorResponse{}

	userID := ctx.GetString("userID")
	ehrID := ctx.Param("ehrid")

	ehrUUID, err := uuid.Parse(ehrID)
	if err != nil {
		errResponse.SetMessage("Incorrect param").
			AddError(errors.ErrFieldIsIncorrect("ehr_id"))

		ctx.JSON(http.StatusBadRequest, errResponse)
		return
	}

	systemID := ctx.GetString("ehrSystemID")
	searcher := helper.NewSearcher(ctx, userID, systemID, ehrUUID.String()).UseService(h.indexer)

	if !searcher.IsEhrBelongsToUser() {
		ctx.AbortWithStatus(http.StatusNotFound)
		return
	}

	d := &model.Directory{}
	if err := json.NewDecoder(ctx.Request.Body).Decode(d); err != nil {
		errResponse.SetMessage("Request body parsing error").AddError(err)

		ctx.JSON(http.StatusBadRequest, errResponse)
		return
	}

	exist, err := h.service.GetByID(ctx, userID, d.UID.Value)
	if err != nil && !errors.Is(err, errors.ErrNotFound) {
		log.Println("directoryService.GetByID error: ", err)
		ctx.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	if exist != nil {
		ctx.AbortWithStatus(http.StatusConflict)
		return
	}

	reqID := ctx.GetString("reqID")

	procRequest, err := h.service.NewProcRequest(reqID, userID, ehrID, processing.RequestDirectoryCreate)
	if err != nil {
		log.Println("directoryService.NewProcRequest error: ", err)
		ctx.AbortWithStatus(http.StatusInternalServerError)
	}

	userInfo, err := h.userService.Info(ctx, userID, systemID)
	if err != nil {
		log.Println("userService.Info error: ", err)
		ctx.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	if err := h.service.Create(ctx, procRequest, systemID, &ehrUUID, userInfo, d); err != nil {
		errResponse.SetMessage("Execute failed").AddError(err)

		ctx.JSON(http.StatusBadRequest, errResponse)
		return
	}

	if err := procRequest.Commit(); err != nil {
		log.Println("Directory procRequest commit error:", err)
		ctx.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	ctx.Header("Location", fmt.Sprintf("%s/%s/directory/%s", h.baseURL, ehrUUID.String(), d.UID.Value))
	ctx.Header("ETag", fmt.Sprintf("\"%s\"", d.UID.Value))

	prefer := ctx.Request.Header.Get("Prefer")
	if prefer == "return=representation" {
		ctx.JSON(http.StatusCreated, d)
		return
	}

	ctx.Status(http.StatusCreated)
}

// Update
// @Summary      Update DIRECTORY
// @Description  https://specifications.openehr.org/releases/ITS-REST/latest/ehr.html#tag/DIRECTORY/operation/directory_update
// @Tags         DIRECTORY
// @Param        Authorization  header    string  true   "Bearer AccessToken"
// @Param        AuthUserId     header    string  true   "UserId UUID"
// @Param        Prefer         header    string     true  "Request header to indicate the preference over response details. The response will contain the entire resource when the Prefer header has a value of return=representation."  Enums: ("return=representation", "return=minimal") default("return=minimal")
// @Param        ehr_id         path      string  true  "EHR identifier taken from EHR.ehr_id.value. Example: 7d44b88c-4199-4bad-97dc-d78268e01398"
// @Header       201            {string}  Etag   "The ETag (i.e. entity tag) response header is the version_uid identifier, enclosed by double quotes. Example: \"8849182c-82ad-4088-a07f-48ead4180515::openEHRSys.example.com::1\""
// @Header       201            {string}  Location   "{baseUrl}/ehr/{ehr_id}/directory/{version_uid}"
// @Header       201            {string}  RequestID  "Request identifier"
// @Accept       json
// @Produce      json
// @Success      200  {object}  model.Directory  "Is returned when the DIRECTORY was successfully updated"
// @Success      204  "Is returned when directory was updated and 'Prefer' header is missing or is set to 'return=minimal'"
// @Failure      400  "Is returned when the request has invalid content"
// @Failure      404  "Is returned when an EHR with {ehr_id} does not exist, or DIRECTORY with that version is not exist"
// @Failure      412  "Is returned when 'If-Match' request header doesn't match the latest version on the service side. Returns also latest 'version_uid' in the 'Location' and 'ETag' headers"
// @Failure      500  "Is returned when an unexpected error occurs while processing a request"
// @Router       /ehr/{ehr_id}/directory [put]
func (h *DirectoryHandler) Update(ctx *gin.Context) {
	errResponse := model.ErrorResponse{}

	userID := ctx.GetString("userID")
	ehrID := ctx.Param("ehrid")

	ehrUUID, err := uuid.Parse(ehrID)
	if err != nil {
		errResponse.SetMessage("Incorrect param").
			AddError(errors.ErrFieldIsIncorrect("ehr_id"))

		ctx.JSON(http.StatusBadRequest, errResponse)
		return
	}

	systemID := ctx.GetString("ehrSystemID")
	searcher := helper.NewSearcher(ctx, userID, systemID, ehrUUID.String()).UseService(h.indexer)

	if !searcher.IsEhrBelongsToUser() {
		ctx.AbortWithStatus(http.StatusNotFound)
		return
	}

	d := &model.Directory{}
	if err := json.NewDecoder(ctx.Request.Body).Decode(d); err != nil {
		errResponse.SetMessage("Request body parsing error").AddError(err)

		ctx.JSON(http.StatusBadRequest, errResponse)
		return
	}

	precedingVersionUID := ctx.GetHeader("If-Match")
	if precedingVersionUID == "" {
		ctx.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "If-Match is empty"})
		return
	}

	lastDirectoryVersion, err := h.service.GetByTime(ctx, systemID, &ehrUUID, userID, time.Now())
	if err != nil {
		if !errors.Is(err, errors.ErrNotFound) {
			log.Println("directoryService.GetByTime error: ", err)
			ctx.AbortWithStatus(http.StatusInternalServerError)
			return
		}

		ctx.AbortWithStatus(http.StatusNotFound)
		return
	}

	if lastDirectoryVersion.UID.Value != d.UID.Value || d.UID.Value != precedingVersionUID {
		ctx.AbortWithStatus(http.StatusPreconditionFailed)
		return
	}

	reqID := ctx.GetString("reqID")

	procRequest, err := h.service.NewProcRequest(reqID, userID, ehrID, processing.RequestDirectoryUpdate)
	if err != nil {
		log.Println("directoryService.NewProcRequest error: ", err)
		ctx.AbortWithStatus(http.StatusInternalServerError)
	}

	userInfo, err := h.userService.Info(ctx, userID, systemID)
	if err != nil {
		log.Println("userService.Info error: ", err)
		ctx.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	if err := h.service.Update(ctx, procRequest, systemID, &ehrUUID, userInfo, d); err != nil {
		errResponse.SetMessage("Execute failed").AddError(err)

		ctx.JSON(http.StatusBadRequest, errResponse)
		return
	}

	if err := procRequest.Commit(); err != nil {
		log.Println("Directory procRequest commit error:", err)
		ctx.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	ctx.Header("Location", fmt.Sprintf("%s/%s/directory/%s", h.baseURL, ehrUUID.String(), d.UID.Value))
	ctx.Header("ETag", fmt.Sprintf("\"%s\"", d.UID.Value))

	prefer := ctx.Request.Header.Get("Prefer")
	if prefer == "return=representation" {
		ctx.JSON(http.StatusOK, d)
		return
	}

	ctx.Status(http.StatusNoContent)
}

// Delete
// @Summary      Delete DIRECTORY folder associated with the EHR identified by ehr_id.
// @Description  https://specifications.openehr.org/releases/ITS-REST/latest/ehr.html#tag/DIRECTORY/operation/directory_delete
// @Description  The existing latest {version_uid} of directory FOLDER resource (i.e. the {preceding_version_uid}) must be specified in the {If-Match} header.
// @Tags         DIRECTORY
// @Param        Authorization  header    string  true   "Bearer AccessToken"
// @Param        AuthUserId     header    string  true   "UserId UUID"
// @Param        Prefer         header    string     true  "Request header to indicate the preference over response details. The response will contain the entire resource when the Prefer header has a value of return=representation."  Enums: ("return=representation", "return=minimal") default("return=minimal")
// @Param        ehr_id         path      string  true  "EHR identifier taken from EHR.ehr_id.value. Example: 7d44b88c-4199-4bad-97dc-d78268e01398"
// @Header       204            {string}  RequestID  "Request identifier"
// @Header       412            {string}  Etag   "The ETag (i.e. entity tag) response header is the version_uid identifier, enclosed by double quotes. Example: \"8849182c-82ad-4088-a07f-48ead4180515::openEHRSys.example.com::1\""
// @Header       412            {string}  Location   "{baseUrl}/ehr/{ehr_id}/directory/{version_uid}"
// @Success      204  "Is returned when the resource identified by the request parameters has been (logically) deleted"
// @Failure      400  "Is returned when the request has invalid content"
// @Failure      404  "Is returned when an EHR with {ehr_id} does not exist, or DIRECTORY with that version is not exist"
// @Failure      412  "Is returned when 'If-Match' request header doesn't match the latest version on the service side. Returns also latest 'version_uid' in the 'Location' and 'ETag' headers"
// @Failure      500  "Is returned when an unexpected error occurs while processing a request"
// @Router       /ehr/{ehr_id}/directory [delete]
func (h *DirectoryHandler) Delete(ctx *gin.Context) {
	errResponse := model.ErrorResponse{}

	userID := ctx.GetString("userID")
	ehrID := ctx.Param("ehrid")

	ehrUUID, err := uuid.Parse(ehrID)
	if err != nil {
		errResponse.SetMessage("Incorrect param").
			AddError(errors.ErrFieldIsIncorrect("ehr_id"))

		ctx.JSON(http.StatusBadRequest, errResponse)
		return
	}

	systemID := ctx.GetString("ehrSystemID")
	searcher := helper.NewSearcher(ctx, userID, systemID, ehrUUID.String()).UseService(h.indexer)

	if !searcher.IsEhrBelongsToUser() {
		ctx.AbortWithStatus(http.StatusNotFound)
		return
	}

	precedingVersionUID := ctx.GetHeader("If-Match")
	if precedingVersionUID == "" {
		ctx.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "If-Match is empty"})
		return
	}

	lastDirectoryVersion, err := h.service.GetByTime(ctx, systemID, &ehrUUID, userID, time.Now())
	if err != nil {
		if !errors.Is(err, errors.ErrNotFound) {
			log.Println("directoryService.GetByTime error: ", err)
			ctx.AbortWithStatus(http.StatusInternalServerError)
			return
		}

		ctx.AbortWithStatus(http.StatusNotFound)
		return
	}

	if lastDirectoryVersion.String() != precedingVersionUID {
		ctx.Header("Location", fmt.Sprintf("%s/%s/directory/%s", h.baseURL, ehrUUID.String(), lastDirectoryVersion.UID.Value))
		ctx.Header("ETag", fmt.Sprintf("\"%s\"", lastDirectoryVersion.UID.Value))
		ctx.AbortWithStatus(http.StatusPreconditionFailed)
		return
	}

	reqID := ctx.GetString("reqID")

	procRequest, err := h.service.NewProcRequest(reqID, userID, ehrID, processing.RequestDirectoryDelete)
	if err != nil {
		log.Println("directoryService.NewProcRequest error: ", err)
		ctx.AbortWithStatus(http.StatusInternalServerError)
	}

	newUUID, err := h.service.Delete(ctx, procRequest, systemID, &ehrUUID, precedingVersionUID, userID)
	if err != nil {
		if errors.Is(err, errors.ErrAlreadyDeleted) {
			errResponse.SetMessage("Execute failed").AddError(err)
			ctx.JSON(http.StatusBadRequest, errResponse)
			return
		}

		log.Println("Delete error:", err)
		ctx.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	if err := procRequest.Commit(); err != nil {
		log.Println("Directory procRequest commit error:", err)
		ctx.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	ctx.Header("Location", fmt.Sprintf("%s/%s/directory/%s", h.baseURL, ehrUUID.String(), newUUID))
	ctx.Header("ETag", fmt.Sprintf("\"%s\"", newUUID))
	ctx.Status(http.StatusNoContent)
}

// Get folder in DIRECTORY
// @Summary      Get folder in DIRECTORY version at time.
// @Description  https://specifications.openehr.org/releases/ITS-REST/latest/ehr.html#tag/DIRECTORY/operation/directory_get_at_time
// @Description  Retrieves the version of the directory FOLDER associated with the EHR identified by {ehr_id}. If {version_at_time} is supplied, retrieves the version extant at specified time, otherwise retrieves the latest directory FOLDER version. If path is supplied, retrieves from the directory only the sub-FOLDER that is associated with that path.
// @Tags         DIRECTORY
// @Param        Authorization  header    string  true   "Bearer AccessToken"
// @Param        AuthUserId     header    string  true   "UserId UUID"
// @Param        Prefer         header    string     true  "Request header to indicate the preference over response details. The response will contain the entire resource when the Prefer header has a value of return=representation."  Enums: ("return=representation", "return=minimal") default("return=minimal")
// @Param        ehr_id         path      string  true  "EHR identifier taken from EHR.ehr_id.value. Example: 7d44b88c-4199-4bad-97dc-d78268e01398"
// @Param        version_at_time  query     string  true  "Example: version_at_time=2015-01-20T19:30:22.765+01:00 A given time in the extended ISO 8601 format"
// @Param        path  query     string  true  "Example: path=episodes/a/b/c A path to a sub-folder; consists of slash-separated values of the name attribute of FOLDERs in the directory"
// @Produce      json
// @Success      200  {object}  model.Directory "Is returned when the FOLDER is successfully retrieved"
// @Success      204  "Is returned when the resource identified by the request parameters (at specified {version_at_time}) time has been deleted"
// @Failure      400  "Is returned when the request has invalid content"
// @Failure      404  "Is returned when an EHR with {ehr_id} does not exist, or when a directory does not exist at the specified {version_at_time}, or when {path} does not exists within the directory"
// @Failure      500  "Is returned when an unexpected error occurs while processing a request"
// @Router       /ehr/{ehr_id}/directory [get]
func (h *DirectoryHandler) GetByTime(ctx *gin.Context) {
	errResponse := model.ErrorResponse{}

	userID := ctx.GetString("userID")
	ehrID := ctx.Param("ehrid")

	ehrUUID, err := uuid.Parse(ehrID)
	if err != nil {
		errResponse.SetMessage("Incorrect param").
			AddError(errors.ErrFieldIsIncorrect("ehr_id"))

		ctx.JSON(http.StatusBadRequest, errResponse)
		return
	}

	systemID := ctx.GetString("ehrSystemID")
	searcher := helper.NewSearcher(ctx, userID, systemID, ehrUUID.String()).UseService(h.indexer)

	if !searcher.IsEhrBelongsToUser() {
		ctx.AbortWithStatus(http.StatusNotFound)
		return
	}

	versionAtTime := ctx.Query("version_at_time")

	statusTime, err := time.Parse(common.OpenEhrTimeFormat, versionAtTime)
	if err != nil {
		errResponse.SetMessage(fmt.Sprintf("Incorrect format of time option, use %s", common.OpenEhrTimeFormat)).
			AddError(errors.ErrFieldIsIncorrect("version_at_time"))

		ctx.JSON(http.StatusBadRequest, errResponse)
		return
	}

	directoryVersion, err := h.service.GetByTime(ctx, systemID, &ehrUUID, userID, statusTime)
	if err != nil {
		if errors.Is(err, errors.ErrNotFound) {
			ctx.AbortWithStatus(http.StatusNotFound)
			return
		} else if errors.Is(err, errors.ErrAlreadyDeleted) {
			ctx.AbortWithStatus(http.StatusNoContent)
			return
		}

		log.Println("directoryService.GetByTime error: ", err)
		ctx.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	path := ctx.Query("path")

	d, err := directoryVersion.GetByPath(path)
	if err != nil {
		ctx.AbortWithStatus(http.StatusNotFound)
		return
	}

	ctx.JSON(http.StatusOK, &d)
}

// Get folder in DIRECTORY version
// @Summary      Get folder in DIRECTORY by version.
// @Description  https://specifications.openehr.org/releases/ITS-REST/latest/ehr.html#tag/DIRECTORY/operation/directory_get_at_time
// @Description  Retrieves a particular version of the directory FOLDER identified by {version_uid} and associated with the EHR identified by {ehr_id}. If {path} is supplied, retrieves from the directory only the sub-FOLDER that is associated with that path.
// @Tags         DIRECTORY
// @Param        Authorization  header    string  true   "Bearer AccessToken"
// @Param        AuthUserId     header    string  true   "UserId UUID"
// @Param        Prefer         header    string     true  "Request header to indicate the preference over response details. The response will contain the entire resource when the Prefer header has a value of return=representation."  Enums: ("return=representation", "return=minimal") default("return=minimal")
// @Param        ehr_id         path      string  true  "EHR identifier taken from EHR.ehr_id.value. Example: 7d44b88c-4199-4bad-97dc-d78268e01398"
// @Param        version_uid    query   string  true  "Example: 6cb19121-4307-4648-9da0-d62e4d51f19b::openEHRSys.example.com::2 VERSION identifier taken from VERSION.uid.value"
// @Param        path  query     string  true  "Example: path=episodes/a/b/c A path to a sub-folder; consists of slash-separated values of the name attribute of FOLDERs in the directory"
// @Produce      json
// @Success      200  {object}  model.Directory "Is returned when the FOLDER is successfully retrieved"
// @Success      204  "Is returned when the resource identified by the request parameters (at specified {version_at_time}) time has been deleted"
// @Failure      400  "Is returned when the request has invalid content"
// @Failure      404  "Is returned when an EHR with {ehr_id} does not exist, or when a directory does not exist at the specified {version_at_time}, or when {path} does not exists within the directory"
// @Failure      500  "Is returned when an unexpected error occurs while processing a request"
// @Router       /ehr/{ehr_id}/directory/{version_uid} [get]
func (h *DirectoryHandler) GetByVersion(ctx *gin.Context) {
	errResponse := model.ErrorResponse{}

	userID := ctx.GetString("userID")
	ehrID := ctx.Param("ehrid")

	ehrUUID, err := uuid.Parse(ehrID)
	if err != nil {
		errResponse.SetMessage("Incorrect param").
			AddError(errors.ErrFieldIsIncorrect("ehr_id"))

		ctx.JSON(http.StatusBadRequest, errResponse)
		return
	}

	systemID := ctx.GetString("ehrSystemID")
	searcher := helper.NewSearcher(ctx, userID, systemID, ehrUUID.String()).UseService(h.indexer)

	if !searcher.IsEhrBelongsToUser() {
		ctx.AbortWithStatus(http.StatusNotFound)
		return
	}

	versionUID := ctx.Param("version_uid")

	directoryVersion, err := h.service.GetByID(ctx, userID, versionUID)
	if err != nil {
		if errors.Is(err, errors.ErrNotFound) {
			ctx.AbortWithStatus(http.StatusNotFound)
			return
		} else if errors.Is(err, errors.ErrAlreadyDeleted) {
			ctx.AbortWithStatus(http.StatusNoContent)
			return
		}

		log.Println("directoryService.GetByTime error: ", err)
		ctx.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	path := ctx.Query("path")

	d, err := directoryVersion.GetByPath(path)
	if err != nil {
		ctx.AbortWithStatus(http.StatusNotFound)
		return
	}

	ctx.JSON(http.StatusOK, &d)
}
