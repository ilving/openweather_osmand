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
	"sort"

	"github.com/llgcode/draw2d"
	"github.com/llgcode/draw2d/draw2dimg"
	"github.com/sirupsen/logrus"
)

const (
	tileSize = 256.0
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
	Weather []struct {
		Main string
	}

	gTextBox textBox
	texts    []textBox
}
type cInfo struct {
	Cod  int
	List []cityInfo
}

type textBox struct {
	top, right, bottom, left float64
	height, width            float64
	s                        string
	cx, cy                   float64
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
	gc.SetFontSize(8)

	hDiff := gc.GetFontSize() / 3
	// draw items from data list
	// ToDo: check for boxes crossing

	// form strings
	for cIndex, c := range data.List {
		city := &data.List[cIndex]

		cy := tileSize - float64(res.Bounds().Dx())*(math.Abs(c.Coord.Lat)-minLat)/(maxLat-minLat)
		cx := float64(res.Bounds().Dy()) * (math.Abs(c.Coord.Lon) - minLon) / (maxLon - minLon)
		res.Set(int(x), int(y), color.Gray{Y: 0})

		// check coords signs and shift drawing points
		if c.Coord.Lon < 0 {
			cx = tileSize - cx
		}
		if c.Coord.Lat < 0 {
			cy = tileSize - cy
		}

		// skip items that's
		if cx < 0 {
			city.gTextBox.cx = -1
			continue
		}
		if cy < 0 {
			city.gTextBox.cy = -1
			continue
		}

		city.gTextBox = textBox{
			top:    math.MaxFloat64,
			left:   math.MaxFloat64,
			width:  0,
			height: 0,
			cx:     cx,
			cy:     cy,
		}
		city.texts = []textBox{} // number of strings to show
	}

	// calc bboxes
	for cIndex, c := range data.List {
		city := &data.List[cIndex]
		if city.gTextBox.cx < 0 || city.gTextBox.cy < 0 {
			continue
		}

		gc.BeginPath()
		if math.Round(c.Main.Temp) == 0 {
			c.Main.Temp = 0.0
		}

		// 0: city name
		// 1: temp and humidyty
		// ToDo:
		// 3: wind speed (+ direction line)
		// 4: weather text?
		city.texts = append(city.texts, textBox{s: c.Name})
		city.texts = append(city.texts, textBox{s: fmt.Sprintf("T: %.0fC H:%.0f%%", c.Main.Temp, c.Main.Humidity)})
		city.texts = append(city.texts, textBox{s: fmt.Sprintf("Wind: %.1fm/s", city.Wind.Speed)})
		if len(city.Weather) > 0 {
			city.texts = append(city.texts, textBox{s: fmt.Sprintf("%s", city.Weather[0].Main)})
		}

		// calc total box dimensions for N lines
		for i := 0; i < len(city.texts); i++ {
			city.texts[i].left, city.texts[i].top, city.texts[i].right, city.texts[i].bottom = gc.GetStringBounds(city.texts[i].s)

			city.texts[i].height = math.Abs(city.texts[i].top) + math.Abs(city.texts[i].bottom)
			city.texts[i].width = math.Abs(city.texts[i].right) - math.Abs(city.texts[i].left)

			city.gTextBox.left = math.Min(city.gTextBox.left, city.texts[i].left)

			// top and bottom not really valid - some pixels are drawen below center line. library bug?
			city.gTextBox.width = math.Max(city.gTextBox.width, city.texts[i].width)
			city.gTextBox.height += city.texts[i].height + hDiff
		}
		city.gTextBox.top = city.gTextBox.cy // + city.texts[0].top
		city.gTextBox.left = city.gTextBox.cx + city.gTextBox.left

		// shift textBox in case it will be over tile limits
		// ToDo: get info from upper tile??
		if city.gTextBox.left+city.gTextBox.width > tileSize {
			city.gTextBox.left -= city.gTextBox.left + city.gTextBox.width - tileSize + 5
		}
		if city.gTextBox.top+city.gTextBox.height > tileSize {
			city.gTextBox.top -= city.gTextBox.top + city.gTextBox.height - tileSize + 5
		}
	}

	intersect := func(a, b textBox) bool {
		if a.bottom < b.top || a.top > b.bottom {
			return false
		}
		if a.right < b.left || a.left > b.right {
			return false
		}
		return true
	}

	// X-sort ----------------------------------------------
	sort.Slice(data.List, func(i, j int) bool {
		cI := data.List[i].gTextBox
		cJ := data.List[j].gTextBox

		return cI.left+cI.width < cJ.left+cJ.width
	})

	// check cross and shift by X
	isCrossing := true
	for k := 0; k < len(data.List) && isCrossing; k++ {
		if len(data.List) == 1 {
			break
		}
		isCrossing = false
		for i := 0; i < len(data.List); i++ {
			for j := i + 1; j < len(data.List); j++ {
				ci := &data.List[i].gTextBox
				cj := &data.List[j].gTextBox

				iBox := textBox{top: ci.top, left: ci.left, bottom: ci.top + ci.height, right: ci.left + ci.width}
				jBox := textBox{top: cj.top, left: cj.left, bottom: cj.top + cj.height, right: cj.left + cj.width}
				if intersect(iBox, jBox) {
					isCrossing = true
					xDiff := iBox.right - jBox.left + 10
					if jBox.right+xDiff < tileSize { // shift J's bound
						cj.left += xDiff
					} else if iBox.left-xDiff > 0 { // shift I's
						ci.left -= xDiff
					} else if iBox.left-xDiff/2 > 0 && jBox.right+xDiff/2 < tileSize { // shift both by diff/2
						cj.left += xDiff / 2
						ci.left -= xDiff / 2
					}
				}
			}
		}
	}
	// X-Sort end --------------------------------------------

	// Y-Sort -----------------------------------------
	sort.Slice(data.List, func(i, j int) bool {
		cI := data.List[i].gTextBox
		cJ := data.List[j].gTextBox

		return cI.top+cI.height < cJ.top+cJ.height
	})

	// check cross and shift by X
	isCrossing = true
	for k := 0; k < len(data.List) && isCrossing; k++ {
		if len(data.List) == 1 {
			break
		}
		isCrossing = false
		for i := 0; i < len(data.List); i++ {
			for j := i + 1; j < len(data.List); j++ {
				ci := &data.List[i].gTextBox
				cj := &data.List[j].gTextBox

				iBox := textBox{top: ci.top, left: ci.left, bottom: ci.top + ci.height, right: ci.left + ci.width}
				jBox := textBox{top: cj.top, left: cj.left, bottom: cj.top + cj.height, right: cj.left + cj.width}
				if intersect(iBox, jBox) {
					isCrossing = true
					yDiff := iBox.bottom - jBox.top + 10
					if jBox.bottom+yDiff < tileSize { // shift J's bound
						cj.top += yDiff
					} else if iBox.top-yDiff > 0 { // shift I's
						ci.top -= yDiff
					} else if iBox.top-yDiff/2 > 0 && jBox.bottom+yDiff/2 < tileSize { // shift both by diff/2
						cj.top += yDiff / 2
						ci.top -= yDiff / 2
					}
				}
			}
		}
	}
	// Y-Sort end

	for cIndex := range data.List {
		city := &data.List[cIndex]
		// draw partially transparent rect
		gc.SetStrokeColor(color.NRGBA{R: 90, G: 90, B: 90, A: 30})
		for sy := -3.0; sy < city.gTextBox.height+3; sy += 1.0 {
			gc.MoveTo(city.gTextBox.left-3, city.gTextBox.top+sy)
			gc.LineTo(city.gTextBox.left+city.gTextBox.width+3, city.gTextBox.top+sy)
		}
		gc.Stroke()

		// draw text
		gc.SetStrokeColor(color.NRGBA{R: 0, G: 0, B: 255, A: 255})
		gc.SetFillColor(color.NRGBA{R: 0, G: 0, B: 255, A: 255})
		yOffset := 0.0
		for i := 0; i < len(city.texts); i++ {
			gc.FillStringAt(city.texts[i].s, city.gTextBox.left, city.gTextBox.top+yOffset+city.texts[i].height)
			yOffset += city.texts[i].height + hDiff
		}
		gc.Stroke()

		gc.SetStrokeColor(color.Gray{Y: 255})
		gc.SetFillColor(color.Gray{Y: 255})
		gc.MoveTo(city.gTextBox.cx-5, city.gTextBox.cy)
		gc.LineTo(city.gTextBox.cx+5, city.gTextBox.cy)

		gc.MoveTo(city.gTextBox.cx, city.gTextBox.cy-5)
		gc.LineTo(city.gTextBox.cx, city.gTextBox.cy+5)

		gc.Stroke()
	}

	gc.Close()
	return res
}
