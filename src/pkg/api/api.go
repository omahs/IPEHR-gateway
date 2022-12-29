package api

import (
	"net/http"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	ginSwagger "github.com/swaggo/gin-swagger"
	"github.com/swaggo/gin-swagger/swaggerFiles"

	"hms/gateway/pkg/config"
	"hms/gateway/pkg/docs/service"
	"hms/gateway/pkg/docs/service/composition"
	contributionService "hms/gateway/pkg/docs/service/contribution"
	"hms/gateway/pkg/docs/service/groupAccess"
	"hms/gateway/pkg/docs/service/query"
	"hms/gateway/pkg/docs/service/template"
	"hms/gateway/pkg/infrastructure"
	userService "hms/gateway/pkg/user/service"
)

// @title        IPEHR Gateway API
// @version      0.2
// @description  The IPEHR Gateway is an openEHR compliant EHR server implementation that stores encrypted medical data in a Filecoin distributed file storage.

// @contact.name   API Support
// @contact.url    https://bsn.si/blockchain
// @contact.email  support@bsn.si

// @license.name  Apache 2.0
// @license.url   http://www.apache.org/licenses/LICENSE-2.0.html

// @host      gateway.ipehr.org
// host      localhost:8080
// @BasePath  /v1

type API struct {
	Ehr         *EhrHandler
	EhrStatus   *EhrStatusHandler
	Composition *CompositionHandler
	Query       *QueryHandler
	Template    *TemplateHandler
	//GroupAccess *GroupAccessHandler
	DocAccess    *DocAccessHandler
	Request      *RequestHandler
	User         *UserHandler
	Contribution *ContributionHandler
}

func New(cfg *config.Config, infra *infrastructure.Infra) *API {
	docService := service.NewDefaultDocumentService(cfg, infra)
	groupAccessService := groupAccess.NewService(docService, cfg.DefaultGroupAccessID, cfg.DefaultUserID)

	templateService := template.NewService(docService)
	queryService := query.NewService(docService)
	user := userService.NewService(infra, docService.Proc)
	contribution := contributionService.NewService(docService)

	compositionService := composition.NewCompositionService(
		docService.Infra.Index,
		docService.Infra.IpfsClient,
		docService.Infra.FilecoinClient,
		docService.Infra.Keystore,
		docService.Infra.Compressor,
		docService,
		groupAccessService)

	return &API{
		Ehr:         NewEhrHandler(docService, cfg.BaseURL),
		EhrStatus:   NewEhrStatusHandler(docService, cfg.BaseURL),
		Composition: NewCompositionHandler(docService, compositionService, cfg.BaseURL),
		Query:       NewQueryHandler(queryService, cfg.BaseURL),
		Template:    NewTemplateHandler(templateService, cfg.BaseURL),
		//GroupAccess: NewGroupAccessHandler(docService, groupAccessService, cfg.BaseURL),
		DocAccess:    NewDocAccessHandler(docService),
		Request:      NewRequestHandler(docService),
		User:         NewUserHandler(user),
		Contribution: NewContributionHandler(contribution, user, templateService, compositionService, cfg.BaseURL),
	}
}

func (a *API) Build() *gin.Engine {
	return a.setupRouter(
		a.buildUserAPI(),
		a.buildEhrAPI(),
		a.buildEhrContributionAPI(),
		a.buildAccessAPI(),
		//a.buildGroupAccessAPI(),
		a.buildQueryAPI(),
		a.buildDefinitionAPI(),
		a.buildRequestsAPI(),
	)
}

type handlerBuilder func(r *gin.RouterGroup)

func (a *API) setupRouter(apiHandlers ...handlerBuilder) *gin.Engine {
	r := gin.New()

	//TODO complete CORS config
	config := cors.DefaultConfig()
	config.AllowAllOrigins = true
	config.AddAllowHeaders(
		"AuthUserId",
		"Authorization",
		"GroupAccessId",
		"EhrSystemId",
		"Prefer",
	)

	r.Use(cors.New(config))

	setRedirections(r)

	r.NoRoute(func(c *gin.Context) {
		c.AbortWithStatus(404)
	})

	// r.Use(gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
	// 	// your custom format
	// 	return fmt.Sprintf("[GIN] %19s | %6s | %3d | %13v | %15s | %-7s %#v %s\n",
	// 		param.TimeStamp.Format("2006-01-02 15:04:05"),
	// 		param.Keys["reqID"],
	// 		param.StatusCode,
	// 		param.Latency,
	// 		param.ClientIP,
	// 		param.Method,
	// 		param.Path,
	// 		param.ErrorMessage,
	// 	)
	// }))

	r.Use(requestID)

	v1 := r.Group("v1")
	for _, b := range apiHandlers {
		b(v1)
	}

	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	return r
}

func (a *API) buildEhrAPI() handlerBuilder {
	return func(r *gin.RouterGroup) {
		r = r.Group("ehr")
		r.Use(gzip.Gzip(gzip.DefaultCompression))
		//r.Use(Recovery, app_errors.ErrHandler)
		r.Use(auth(a))
		r.Use(ehrSystemID)
		r.POST("", a.Ehr.Create)
		r.GET("", a.Ehr.GetBySubjectIDAndNamespace)
		r.PUT("/:ehrid", a.Ehr.CreateWithID)
		r.GET("/:ehrid", a.Ehr.GetByID)
		r.PUT("/:ehrid/ehr_status", a.EhrStatus.Update)
		r.GET("/:ehrid/ehr_status/:versionid", a.EhrStatus.GetByID)
		r.GET("/:ehrid/ehr_status", a.EhrStatus.GetStatusByTime)
		r.POST("/:ehrid/composition", a.Composition.Create)
		r.GET("/:ehrid/composition", a.Composition.GetList)
		r.GET("/:ehrid/composition/:version_uid", a.Composition.GetByID)
		r.DELETE("/:ehrid/composition/:preceding_version_uid", a.Composition.Delete)
		r.PUT("/:ehrid/composition/:versioned_object_uid", a.Composition.Update)
	}
}

func (a *API) buildEhrContributionAPI() handlerBuilder {
	return func(r *gin.RouterGroup) {
		r = r.Group("ehr")
		r.Use(gzip.Gzip(gzip.DefaultCompression))
		r.Use(auth(a))
		r.Use(ehrSystemID)
		r.GET("/:ehrid/contribution/:contribution_uid", a.Contribution.GetByID)
		r.POST("/:ehrid/contribution/", a.Contribution.Create)
	}
}

func (a *API) buildAccessAPI() handlerBuilder {
	return func(r *gin.RouterGroup) {
		r = r.Group("access")
		r.Use(auth(a))
		//r.GET("/group/:group_id", a.GroupAccess.Get)
		//r.POST("/group", a.GroupAccess.Create)

		r.POST("/document", a.DocAccess.Set)
		r.GET("/document/", a.DocAccess.List)
	}
}

func (a *API) buildQueryAPI() handlerBuilder {
	return func(r *gin.RouterGroup) {
		r = r.Group("query")
		r.Use(auth(a))
		r.POST("/aql", a.Query.ExecPost)
	}
}

func (a *API) buildDefinitionAPI() handlerBuilder {
	return func(r *gin.RouterGroup) {
		r = r.Group("definition")

		r.Use(auth(a))
		r.Use(ehrSystemID)

		adlV1 := r.Group("template/adl1.4")
		adlV1.GET("/:template_id", a.Template.GetByID)
		adlV1.GET("", a.Template.ListStored)
		adlV1.POST("", a.Template.Store)

		query := r.Group("query")
		query.GET("/:qualified_query_name", a.Query.ListStored)
		query.GET("/:qualified_query_name/:version", a.Query.GetStoredByVersion)
		query.PUT("/:qualified_query_name", a.Query.Store)
		query.PUT("/:qualified_query_name/:version", a.Query.StoreVersion)
	}
}

func (a *API) buildRequestsAPI() handlerBuilder {
	return func(r *gin.RouterGroup) {
		r = r.Group("requests")
		r.Use(auth(a, "userRegister"))
		r.GET("/:reqID", a.Request.GetByID)
		r.GET("/", a.Request.GetAll)
	}
}

func (a *API) buildUserAPI() handlerBuilder {
	return func(r *gin.RouterGroup) {
		r = r.Group("user")
		r.Use(gzip.Gzip(gzip.DefaultCompression))

		r.GET("/:user_id", a.User.Info)
		r.GET("/code/:code", a.User.InfoByCode)

		r.Use(ehrSystemID)

		r.POST("/register", a.User.Register)
		r.POST("/login", a.User.Login)
		r.GET("/refresh", a.User.RefreshToken)

		r.Use(auth(a))
		r.POST("/logout", a.User.Logout)

		r = r.Group("group")
		r.POST("", a.User.GroupCreate)
		r.GET("", a.User.GroupGetList)
		r.GET("/:group_id", a.User.GroupGetByID)
		r.PUT("/:group_id/user_add/:user_id/:access_level", a.User.GroupAddUser)
		r.POST("/:group_id/user_remove/:user_id", a.User.GroupRemoveUser)
	}
}

func setRedirections(r *gin.Engine) *gin.Engine {
	redirect := func(c *gin.Context) {
		c.Redirect(http.StatusSeeOther, "v1/")
	}

	r.GET("/", redirect)
	r.HEAD("/", redirect)

	return r
}
