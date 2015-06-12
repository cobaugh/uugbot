/*
Licensed to the Apache Software Foundation (ASF) under one
or more contributor license agreements.  See the NOTICE file
distributed with this work for additional information
regarding copyright ownership.  The ASF licenses this file
to you under the Apache License, Version 2.0 (the
"License"); you may not use this file except in compliance
with the License.  You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing,
software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
KIND, either express or implied.  See the License for the
specific language governing permissions and limitations
under the License.
*/

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/karlseguin/rcache"
	"github.com/nickvanw/ircx"
	"github.com/sorcix/irc"
)

type PlaceInfo struct {
	PlaceName string  `json:"place name"`
	State     string  `json:"state"`
	StateAbbr string  `json:"state abbreviation"`
	Latitude  float64 `json:"latitude,string"`
	Longitude float64 `json:"longitude,string"`
}

type ZipInfo struct {
	PostCode    string      `json:"post code"`
	Country     string      `json:"country"`
	CountryAbbr string      `json:"country abbreviation"`
	Places      []PlaceInfo `json:"places"`
}

type Current struct {
	Time                 int64   `json:"time"`
	Summary              string  `json:"summary"`
	Icon                 string  `json:"icon"`
	NearestStormDistance float64 `json:"nearestStormDistance"`
	NearestStormBearing  float64 `json:"nearestStormBearing"`
	PrecipIntensity      float64 `json:"precipIntensity"`
	PrecipProbability    float64 `json:"precipProbability"`
	Temperature          float64 `json:"temperature"`
	ApparentTemperature  float64 `json:"apparentTemperature"`
	DewPoint             float64 `json:"dewPoint"`
	Humidity             float64 `json:"humidity"`
	WindSpeed            float64 `json:"windSpeed"`
	WindBearing          float64 `json:"windBearing"`
	Visibility           float64 `json:"visibility"`
	CloudCover           float64 `json:"cloudCover"`
	Pressure             float64 `json:"pressure"`
	Ozone                float64 `json:"ozone"`
}

type Minutely struct {
	Summary string `json:"summary"`
}

type Hourly struct {
	Summary string `json:"summary"`
}

type Daily struct {
	Summary string `json:"summary"`
}

type WeatherReport struct {
	Latitude  float64  `json:"latitude"`
	Longitude float64  `json:"longitude"`
	Timezone  string   `json:"timezone"`
	Offset    float64  `json:"offset"`
	Currently Current  `json:"currently"`
	Minutely  Minutely `json:"minutely"`
	Hourly    Hourly   `json:"hourly"`
	Daily     Daily    `json:"daily"`
}

var cache *rcache.Cache

func fetcher(key string) interface{} {
	var z ZipInfo

	log.Println("Looking up coordinates for zip:", key)
	resp, err := http.Get(fmt.Sprintf("http://api.zippopotam.us/us/%s", key))

	if err != nil {
		log.Printf("Lookup failed for zip: %s (%s)\n", key, err)
		return nil
	}

	defer resp.Body.Close()
	dec := json.NewDecoder(resp.Body)
	err = dec.Decode(&z)

	if err != nil {
		log.Printf("Unable to parse result for zip: %s (%s)\n", key, err)
		return nil
	}

	return &z
}

func init() {
	cache = rcache.New(fetcher, time.Hour*24*7)
}

func GetWeather(s ircx.Sender, message *irc.Message) {
	if len(message.Trailing) == 5 {
		if _, err := strconv.Atoi(message.Trailing); err == nil {
			p := message.Params
			if p[0] == config.General.Name {
				p = []string{message.Prefix.Name}
			}

			m := &irc.Message{
				Command: irc.PRIVMSG,
				Params:  p,
			}

			zl := cache.Get(message.Trailing)

			if zl != nil {
				z := zl.(*ZipInfo)
				if z.Places != nil {
					resp, err := http.Get(fmt.Sprint("https://api.forecast.io/forecast/", config.Forecast.Key, "/",
						z.Places[0].Latitude, ",", z.Places[0].Longitude, "?exclude=flags"))
					if err != nil {
						// handle error
						return
					}
					defer resp.Body.Close()

					dec := json.NewDecoder(resp.Body)

					var w WeatherReport
					err = dec.Decode(&w)

					l, _ := time.LoadLocation(w.Timezone)

					t := time.Unix(w.Currently.Time, 0).In(l)

					log.Println("Sending weather for", message.Trailing)

					m.Trailing = fmt.Sprint(message.Prefix.Name, ": ", z.Places[0].PlaceName, ", ", z.Places[0].StateAbbr,
						" (", z.Places[0].Latitude, ", ", z.Places[0].Longitude, ") ", t, " - ",
						w.Currently.Temperature, "F (feels like ", w.Currently.ApparentTemperature, "F) - ",
						w.Currently.Summary)
					s.Send(m)

					m.Trailing = fmt.Sprint(message.Prefix.Name, ": ",
						w.Currently.Humidity*100, "% Humidity - ",
						"Wind from ", w.Currently.WindBearing, "° at ", w.Currently.WindSpeed, "MPH - ",
						"Visibility ", w.Currently.Visibility, " Miles - ",
						"Cloud Cover ", w.Currently.CloudCover*100, "% - ",
						"Precipitation Probability ", w.Currently.PrecipProbability*100, "%")
					s.Send(m)

					m.Trailing = fmt.Sprint(message.Prefix.Name, ": ", w.Minutely.Summary, " ", w.Hourly.Summary, " ", w.Daily.Summary)
					s.Send(m)
				} else {
					log.Println("No data returned for zip:", message.Trailing)
				}
			}
		}
	}
}
