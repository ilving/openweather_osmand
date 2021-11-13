package owm

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"net/http"
	"os"

	"github.com/llgcode/draw2d"
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
	Main struct {
		Temp     float64
		Humidity float64
	}
	Wind struct {
		Speed float64
		Deg   float64
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

	logrus.Info(url)

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
	cbbox := v.latlon(x, y, z)
	data := v.getData(cbbox)

	// copy src image to output
	res := image.NewRGBA(image.Rect(0, 0, src.Bounds().Dx(), src.Bounds().Dy()))
	draw.Draw(res, res.Bounds(), src, res.Bounds().Min, draw.Src)

	minLat := math.Min(math.Abs(cbbox.lat1), math.Abs(cbbox.lat0))
	maxLat := math.Max(math.Abs(cbbox.lat1), math.Abs(cbbox.lat0))

	minLon := math.Min(math.Abs(cbbox.lon0), math.Abs(cbbox.lon1))
	maxLon := math.Max(math.Abs(cbbox.lon0), math.Abs(cbbox.lon1))

	// get font
	wd, _ := os.Getwd()
	gc := draw2dimg.NewGraphicContext(res)
	draw2d.SetFontFolder(wd + "/fonts")

	gc.SetFontData(draw2d.FontData{Name: "luxi", Family: draw2d.FontFamilyMono, Style: draw2d.FontStyleNormal})
	gc.SetFontSize(10)

	// draw items from data list
	// ToDo: check for boxes crossing

	for _, c := range data.List {
		cy := 256 - float64(res.Bounds().Dx())*(math.Abs(c.Coord.Lat)-minLat)/(maxLat-minLat)
		cx := float64(res.Bounds().Dy()) * (math.Abs(c.Coord.Lon) - minLon) / (maxLon - minLon)
		res.Set(int(x), int(y), color.Gray{Y: 0})

		// check coords signs and shift drawing points
		if c.Coord.Lon < 0 {
			cx = 256 - cx
		}
		if c.Coord.Lat < 0 {
			cy = 256 - cy
		}

		// skip items that's
		if cx < 0 {
			continue
		}
		if cy < 0 {
			continue
		}
		gc.BeginPath()

		type textBox struct {
			top, right, bottom, left float64
			height, width            float64
			s                        string
		}

		boxes := [2]textBox{}
		bTextBox := textBox{right: -math.MaxFloat64, left: math.MaxFloat64}

		// 0: city name
		// 1: temp and humidyty
		// ToDo:
		// 3: wind speed (+ direction line)
		// 4: weather text?
		boxes[0].s = c.Name
		boxes[1].s = fmt.Sprintf("T: %.0fC H:%.0f%%", c.Main.Temp, c.Main.Humidity)

		// calc total box dimensions for N lines
		for i := 0; i < len(boxes); i++ {
			boxes[i].left, boxes[i].top, boxes[i].right, boxes[i].bottom = gc.GetStringBounds(boxes[i].s)
			boxes[i].height = math.Abs(boxes[i].top) + math.Abs(boxes[i].bottom)
			boxes[i].width = boxes[i].right - boxes[i].left

			bTextBox.right = math.Max(bTextBox.right, boxes[i].right)
			bTextBox.left = math.Min(bTextBox.left, boxes[i].left)

			// top and bottom not really valid - some pixels are drawen below center line. library bug?
			bTextBox.height += boxes[i].height + gc.GetFontSize()/2
			bTextBox.bottom += boxes[i].height
		}
		bTextBox.top = boxes[0].top
		bTextBox.bottom += boxes[0].top

		bTextBox.width = bTextBox.right - bTextBox.left

		tx := cx
		ty := cy

		// shift textBox in case it will be over tile limits
		// ToDo: get info from upper tile??
		if tx+bTextBox.width > 256 {
			tx -= tx + bTextBox.width - 256 + 2
		}
		if ty+bTextBox.height > 256 {
			ty -= ty + bTextBox.height - 256 + 2
		}

		bsx := tx
		bsy := ty

		// draw partially transparent rect
		gc.SetStrokeColor(color.NRGBA{R: 90, G: 90, B: 90, A: 30})
		for sy := -3.0; sy < bTextBox.height+3; sy += 1.0 {
			gc.MoveTo(bsx-3, bsy+sy)
			gc.LineTo(bsx+bTextBox.width+3, bsy+sy)
		}
		gc.Stroke()

		// draw text
		gc.SetStrokeColor(color.NRGBA{R: 0, G: 0, B: 255, A: 255})
		gc.SetFillColor(color.NRGBA{R: 0, G: 0, B: 255, A: 255})
		yOffset := bTextBox.top + boxes[0].height
		for i := 0; i < len(boxes); i++ {
			gc.FillStringAt(boxes[i].s, tx, ty+yOffset+boxes[i].height)
			yOffset += boxes[i].height + gc.GetFontSize()/3
		}
		gc.Stroke()

		gc.SetStrokeColor(color.Gray{Y: 255})
		gc.MoveTo(cx-5, cy)
		gc.LineTo(cx+5, cy)

		gc.MoveTo(cx, cy-5)
		gc.LineTo(cx, cy+5)

		gc.Stroke()
	}

	gc.Close()
	return res
}
