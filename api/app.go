package api

import (
	v1 "crynux_bridge/api/v1"
	responseV1 "crynux_bridge/api/v1/response"
	"crynux_bridge/config"
	"reflect"
	"regexp"
	"strings"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
	"github.com/loopfz/gadgeto/tonic"
	log "github.com/sirupsen/logrus"
	ginlogrus "github.com/toorop/gin-logrus"
	"github.com/wI2L/fizz"
	"github.com/wI2L/fizz/openapi"
)

func GetHttpApplication(appConfig *config.AppConfig) *gin.Engine {

	gin.SetMode(appConfig.Environment)

	engine := gin.New()
	corsConfig := cors.DefaultConfig()
	corsConfig.AllowAllOrigins = true
	corsConfig.AddAllowHeaders("Authorization")
	engine.Use(cors.New(corsConfig))
	engine.Use(ginlogrus.Logger(log.StandardLogger()), gin.Recovery())
	engine.Use(APIVersion())

	// Serve static files under static folder
	// for OpenAPI documentations
	engine.Use(static.Serve("/static", static.LocalFile("./static", false)))

	// Serve model images
	engine.Use(static.Serve("/images/models", static.LocalFile(appConfig.DataDir.ModelImages, false)))

	fizzEngine := fizz.NewFromEngine(engine)

	// Do not include package name in component names
	fizzEngine.Generator().UseFullSchemaNames(false)

	// Initialize our own handlers
	tonic.SetBindHook(tonic.DefaultBindingHookMaxBodyBytes(appConfig.Http.MaxBodyBytes))
	tonic.SetErrorHook(TonicResponseErrorHook)
	tonic.SetRenderHook(TonicRenderHook, "")
	tonic.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
		if name == "-" {
			return ""
		}
		return name
	})

	// v1 api
	v1.InitRoutes(fizzEngine)

	// Serve OpenAPI specifications
	infos := &openapi.Info{
		Title:       "H Relay",
		Description: "The relay server for the Crynux Node in the Hydrogen(H) Network",
		Version:     "1.0.0",
	}

	fizzEngine.GET("/openapi.json", nil, fizzEngine.OpenAPI(infos, "json"))
	fizzEngine.GET("/openapi.yml", nil, fizzEngine.OpenAPI(infos, "yaml"))

	if len(fizzEngine.Errors()) != 0 {

		for err := range fizzEngine.Errors() {
			log.Error(err)
		}

		panic("fizz initialization error")
	}

	return engine
}

func APIVersion() gin.HandlerFunc {
	return func(c *gin.Context) {

		path := c.FullPath()

		re := regexp.MustCompile(`^/v([0-9]+)/`)
		matches := re.FindStringSubmatch(path)

		if len(matches) > 1 {
			c.Set("api_version", matches[1])
		}

		c.Next()
	}
}

// TonicResponseErrorHook Distribute binding & error handling & render handling to implementations in different API versions
func TonicResponseErrorHook(ctx *gin.Context, err error) (int, interface{}) {
	apiVersion := ctx.GetString("api_version")
	switch apiVersion {
	case "1":
		return responseV1.TonicErrorResponse(ctx, err)
	default:
		return tonic.DefaultErrorHook(ctx, err)
	}
}

func TonicRenderHook(ctx *gin.Context, statusCode int, payload interface{}) {
	apiVersion := ctx.GetString("api_version")
	switch apiVersion {
	case "1":
		responseV1.TonicRenderResponse(ctx, statusCode, payload)
	default:
		tonic.DefaultRenderHook(ctx, statusCode, payload)
	}
}
