// Following https://howistart.org/posts/go/1/

package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"
)

var weatherUndergroundAPIKey = "991e0d84bd9e404a9e0d84bd9ef04a0d"

type weatherProvider interface {
	temperature(city string) (float64, error)
}
type multiWeatherProvider []weatherProvider

type openWeatherMap struct{}
type weatherUnderground struct {
	apiKey string
}

func (w openWeatherMap) temperature(city string) (float64, error) {
	resp, err := http.Get("http://api.openweathermap.org/data/2.5/weather?APPID=ea199eb3a8d6d30f838275b1c7b58042&q=" + city)
	if err != nil {
		return 0, err
	}

	defer resp.Body.Close()

	var data struct {
		Main struct {
			Kelvin float64 `json:"temp"`
		} `json:"main"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return 0, err
	}

	log.Printf("openWeatherMap: %s: %.2f", city, data.Main.Kelvin)
	return data.Main.Kelvin, nil
}

func (w weatherUnderground) temperature(city string) (float64, error) {
	resp, err := http.Get("http://api.wunderground.com/api/" + w.apiKey + "/conditions/q/" + city + ".json")
	if err != nil {
		return 0, err
	}

	defer resp.Body.Close()

	var data struct {
		Observation struct {
			Celsius float64 `json:"temp_c"`
		} `json:"current_observation"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return 0, err
	}

	kelvin := data.Observation.Celsius + 273.15
	log.Printf("weatherUnderground: %s: %.2f", city, kelvin)
	return kelvin, nil
}

func (w multiWeatherProvider) temperature(city string) (float64, error) {
	sum := 0.0

	for _, provider := range w {
		k, err := provider.temperature(city)
		if err != nil {
			return 0, err
		}

		sum += k
	}

	return sum / float64(len(w)), nil
}

func hello(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("hello!"))
}

func main() {
	mw := multiWeatherProvider{
		openWeatherMap{},
		weatherUnderground{apiKey: weatherUndergroundAPIKey},
	}

	http.HandleFunc("/", hello)

	http.HandleFunc("/weather/", func(w http.ResponseWriter, r *http.Request) {
		begin := time.Now()
		city := strings.SplitN(r.URL.Path, "/", 3)[2]

		temp, err := mw.temperature(city)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"city": city,
			"temp": temp,
			"took": time.Since(begin).String(),
		})
	})

	http.ListenAndServe(":8080", nil)
}
