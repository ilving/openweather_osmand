package main

import (
	"flag"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"

	"github.com/ilving/openweather_osmand/owm"
)

var (
	host        string = ""
	apiKey      string
	listen      string
	logFileName string
)

type cacheItem struct {
	img image.Image
	ts  time.Time
}

type owmHandler struct {
	tileService   owm.Tileable
	vectorService owm.Vector
	cache         map[string]cacheItem
}

func main() {
	flag.StringVar(&listen, "listen", "0.0.0.0:9093", "")
	flag.StringVar(&logFileName, "log", "/tmp/owm.log", "")
	flag.StringVar(&apiKey, "key", "zzz", "")
	flag.Parse()

	h := &owmHandler{
		tileService:   owm.NewTileSource("http://maps.openweathermap.org", apiKey),
		vectorService: owm.NewVectorService("http://api.openweathermap.org", apiKey),
		cache:         make(map[string]cacheItem),
	}

	r := mux.NewRouter()
	r.HandleFunc("/{z:[0-9]+}/{x:[0-9]+}/{y:[0-9]+}", h.web)
	r.HandleFunc("/favicon.ico", func(w http.ResponseWriter, req *http.Request) { w.WriteHeader(http.StatusBadRequest) })

	if err := http.ListenAndServe(listen, r); err != nil {
		logrus.WithError(err).WithField("server", listen).Panic("Failed ListenAndServe")
	}
}

func (h *owmHandler) web(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	var x, y, z uint16
	fmt.Sscanf(fmt.Sprintf("%s %s %s", vars["x"], vars["y"], vars["z"]), "%d %d %d", &x, &y, &z)

	w.Header().Add("Content-type", "image/png")
	var img image.Image

	cacheKey := fmt.Sprintf("%d|%d|%d", z, x, y)

	if cimg, ok := h.cache[cacheKey]; ok && cimg.ts.Add(5*time.Minute).After(time.Now().UTC()) {
		img = cimg.img
	} else {
		logrus.WithField("tile", vars).Info("RQ")

		img = h.tileService.GetTile(x, y, z)
		if img == nil {
			return
		}
		img = h.vectorService.AddInfo(x, y, z, img)
		if img == nil {
			return
		}

		h.cache[cacheKey] = cacheItem{
			img: img,
			ts:  time.Now().UTC(),
		}

	}

	if img != nil {
		png.Encode(w, img)
	}
}
