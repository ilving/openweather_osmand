package owm

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"net/http"

	"github.com/llgcode/draw2d/draw2dimg"
	"github.com/sirupsen/logrus"
)

type Vector interface {
	AddInfo(x, y, z uint16, src image.Image) image.Image
}

type vectorService struct {
	host   string
	apikey string
}

type bbox struct {
	lat0 float64
	lon0 float64
	lat1 float64
	lon1 float64
	z    uint16
}

type cityInfo struct {
	Name  string
	Coord struct {
		Lon float64
		Lat float64
	}
}
type cInfo struct {
	Cod  int
	List []cityInfo
}

func NewVectorService(host, apikey string) Vector {
	return &vectorService{host: host, apikey: apikey}
}

func (v *vectorService) latlon(x, y, z uint16) bbox {
	tile2lon := func(x, z uint16) float64 {
		var n float64 = math.Pow(2, float64(z))
		return (float64(x) * 360.0 / n) - 180.0
	}
	tile2lat := func(y, z uint16) float64 {
		var n float64 = math.Pow(2, float64(z))
		var s float64 = math.Pi * (1.0 - 2.0*float64(y)/n)
		return math.Atan(math.Sinh(s)) * 180.0 / math.Pi
	}

	return bbox{
		lon0: tile2lon(x, z),
		lat0: tile2lat(y, z),
		lon1: tile2lon(x+1, z),
		lat1: tile2lat(y+1, z),
		z:    z,
	}
}
func (v *vectorService) getData(bbox bbox) cInfo {
	//api.openweathermap.org/data/2.5/box/city?bbox={bbox}&appid={API key}
	url := fmt.Sprintf("%s/data/2.5/box/city?bbox=%.5f,%.5f,%.5f,%.5f,%d&mode=json&units=metric&lang=ru&appid=%s",
		v.host,
		bbox.lon0, bbox.lat0, bbox.lon1, bbox.lat1, bbox.z,
		v.apikey,
	)

	data := cInfo{}
	resp, err := http.Get(url)
	if err != nil {
		logrus.WithError(err).Error("data")
		return data
	}
	defer resp.Body.Close()
	json.NewDecoder(resp.Body).Decode(&data)
	return data
}

func (v *vectorService) AddInfo(x, y, z uint16, src image.Image) image.Image {

	bbox := v.latlon(x, y, z)
	data := v.getData(bbox)

	res := image.NewRGBA(image.Rect(0, 0, src.Bounds().Dx(), src.Bounds().Dy()))
	draw.Draw(res, res.Bounds(), src, res.Bounds().Min, draw.Src)

	minLat := math.Min(bbox.lat1, bbox.lat0)
	maxLat := math.Max(bbox.lat1, bbox.lat0)

	minLon := math.Min(bbox.lon0, bbox.lon1)
	maxLon := math.Max(bbox.lon0, bbox.lon1)

	gc := draw2dimg.NewGraphicContext(res)
	for _, c := range data.List {
		x := float64(res.Bounds().Dx()) * (c.Coord.Lat - minLat) / (maxLat - minLat)
		y := float64(res.Bounds().Dy()) * (c.Coord.Lon - minLon) / (maxLon - minLon)
		res.Set(int(x), int(y), color.Gray{Y: 0})

		gc.BeginPath()
		gc.SetLineWidth(2)
		gc.SetFillColor(color.Gray{Y: 0})
		gc.MoveTo(x-5, y)
		gc.LineTo(x+5, y)
		gc.MoveTo(x, y-5)
		gc.LineTo(x, y+5)
		gc.Stroke()
	}

	gc.Close()
	return res
}
