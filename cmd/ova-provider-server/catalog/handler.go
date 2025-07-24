package catalog

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	liberr "github.com/kubev2v/forklift/pkg/lib/error"
	"github.com/kubev2v/forklift/pkg/lib/logging"
	"gopkg.in/yaml.v2"
)

const Collection = "/catalog"

type Status struct {
	CreatedAt *time.Time `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at"`
	Source    string     `json:"source"`
}

type Handler struct {
	CatalogPath string
	Log         logging.LevelLogger
	ConfigPath  string
	Config      OVAConfig
}

func (r *Handler) AddRoutes(e *gin.Engine) {
	e.GET(Collection, r.List)
}

func (r *Handler) List(ctx *gin.Context) {
	config, err := ReadConfig(r.ConfigPath)
	if err != nil {
		_ = ctx.Error(err)
		return
	}
	for _, source := range config.Sources {

	}
}

func ReadConfig(path string) (config OVAConfig, err error) {
	file, err := os.Open(path)
	if err != nil {
		err = liberr.Wrap(err)
		return
	}
	defer func() {
		_ = file.Close()
	}()
	decoder := yaml.NewDecoder(file)
	err = decoder.Decode(&config)
	if err != nil {
		err = liberr.Wrap(err)
		return
	}
	return
}
