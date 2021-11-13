package owm

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/png"
	"net/http"
	"sync"

	"github.com/sirupsen/logrus"
)

type Tileable interface {
	GetTile(x, y, z uint16) image.Image
}

type tileService struct {
	host   string
	apikey string
}

func NewTileSource(host, apikey string) Tileable {
	return &tileService{host: host, apikey: apikey}
}

func (t *tileService) pr0(x, y, z uint16) image.Image {
	url := fmt.Sprintf("%s/maps/2.0/weather/PR0/%d/%d/%d?appid=%s&opacity=0", t.host, z, x, y, t.apikey)
	pr0, err := http.Get(url)
	if err != nil {
		logrus.WithError(err).Error("Failed to fetch PR0")
		return nil
	}
	defer pr0.Body.Close()

	imgPR0, _, err := image.Decode(pr0.Body)
	if err != nil {
		logrus.WithError(err).Error("Decode PR0 fails")
		return nil
	}

	return imgPR0
}

func (t *tileService) wnd(x, y, z uint16) image.Image {
	url := fmt.Sprintf("%s/maps/2.0/weather/WND/%d/%d/%d?appid=%s&arrow_step=64&use_norm=true", t.host, z, x, y, t.apikey)
	pr0, err := http.Get(url)
	if err != nil {
		logrus.WithError(err).Error("Failed to fetch WND")
		return nil
	}
	defer pr0.Body.Close()

	imgPR0, _, err := image.Decode(pr0.Body)
	if err != nil {
		logrus.WithError(err).Error("Decode WND fails")
		return nil
	}

	return imgPR0
}

func (t *tileService) GetTile(x, y, z uint16) image.Image {
	// http://maps.openweathermap.org/maps/2.0/weather/{op}/{z}/{x}/{y}?appid={API key}

	var pr0, wnd image.Image
	w := sync.WaitGroup{}
	w.Add(2)
	go func() { pr0 = t.pr0(x, y, z); w.Done() }()
	go func() { wnd = t.wnd(x, y, z); w.Done() }()
	w.Wait()

	if pr0 == nil || wnd == nil {
		return nil
	}

	res := image.NewRGBA(image.Rect(0, 0, pr0.Bounds().Dx(), pr0.Bounds().Dy()))
	draw.Draw(res, res.Bounds(), pr0, res.Bounds().Min, draw.Src)

	for x := 0; x < res.Rect.Dx(); x++ {
		for y := 0; y < res.Rect.Dy(); y++ {
			c := color.GrayModel.Convert(wnd.At(x, y)).(color.Gray)
			if c.Y == 0 {
				res.Set(x, y, wnd.At(x, y))
			}
		}
	}

	return res
}
