package main

import (
	"encoding/xml"
	"fmt"
	"strings"
	"time"

	"github.com/parnurzeal/gorequest"
)

var modeNameMappings = map[string]int32{
	"Off":  0,
	"Heat": 1,
	"Cool": 2,
	"Auto": 3,
}
var modeValMappings = []string{"Off", "Heat", "Cool", "Auto"}

var stateMappings = map[string]int32{
	"Off":         0,
	"Heat-Stage1": 1,
	"Heat-Stage2": 1,
	"Cool-Stage1": 2,
	"Cool-Stage2": 2,
}

// TODO Support case where the thermostat is configured to use Celsius

type Pelican struct {
	username string
	password string
	name     string
	target   string
	timezone *time.Location
	req      *gorequest.SuperAgent
}

type PelicanStatus struct {
	Temperature     float64 `msgpack:"temperature"`
	RelHumidity     float64 `msgpack:"relative_humidity"`
	HeatingSetpoint float64 `msgpack:"heating_setpoint"`
	CoolingSetpoint float64 `msgpack:"cooling_setpoint"`
	Override        bool    `msgpack:"override"`
	Fan             bool    `msgpack:"fan"`
	Mode            int32   `msgpack:"mode"`
	State           int32   `msgpack:"state"`
	Time            int64   `msgpack:"time"`
}

// Thermostat Object API Result Structs
type apiResult struct {
	Thermostat apiThermostat `xml:"Thermostat"`
	Success    int32         `xml:"success"`
	Message    string        `xml:"message"`
}

type apiThermostat struct {
	Temperature     float64 `xml:"temperature"`
	RelHumidity     int32   `xml:"humidity"`
	HeatingSetpoint int32   `xml:"heatSetting"`
	CoolingSetpoint int32   `xml:"coolSetting"`
	SetBy           string  `xml:"setBy"`
	HeatNeedsFan    string  `xml:"HeatNeedsFan"`
	System          string  `xml:"system"`
	RunStatus       string  `xml:"runStatus"`
	StatusDisplay   string  `xml:"statusDisplay"`
}

// Thermostat History Object API Result Structs
type apiResultHistory struct {
	XMLName xml.Name   `xml:"result"`
	Success int        `xml:"success"`
	Message string     `xml:"message"`
	Records apiRecords `xml:"ThermostatHistory"`
}

type apiRecords struct {
	Name    string       `xml:"name"`
	History []apiHistory `xml:"History"`
}

type apiHistory struct {
	TimeStamp string `xml:"timestamp"`
}

// Thermostat Site Object API Result Structs
type apiResultSite struct {
	XMLName   xml.Name    `xml:"result"`
	Success   int         `xml:"success"`
	Attribute apiTimezone `xml:"attribute"`
}

type apiTimezone struct {
	Timezone string `xml:"timeZone"`
}

// Miscellaneous Structs
type pelicanStateParams struct {
	HeatingSetpoint *float64
	CoolingSetpoint *float64
	Override        *float64
	Mode            *float64
	Fan             *float64
}

type thermostatInfo struct {
	Name        string `xml:"name"`
	Description string `xml:"description"`
}

type discoverApiResult struct {
	Thermostats []thermostatInfo `xml:"Thermostat"`
	Success     int32            `xml:"success"`
	Message     string           `xml:"message"`
}

func NewPelican(username, password, sitename, name string, timezone *time.Location) *Pelican {
	return &Pelican{
		username: username,
		password: password,
		target:   fmt.Sprintf("https://%s.officeclimatecontrol.net/api.cgi", sitename),
		name:     name,
		req:      gorequest.New(),
		timezone: timezone,
	}
}

func DiscoverPelicans(username, password, sitename string) ([]*Pelican, error) {
	target := fmt.Sprintf("https://%s.officeclimatecontrol.net/api.cgi", sitename)
	resp, _, errs := gorequest.New().Get(target).
		Param("username", username).
		Param("password", password).
		Param("request", "get").
		Param("object", "Thermostat").
		Param("value", "name;description").
		End()
	if errs != nil {
		return nil, fmt.Errorf("Error retrieving thermostat name from %s: %s", target, errs)
	}

	defer resp.Body.Close()
	var result discoverApiResult
	dec := xml.NewDecoder(resp.Body)
	if err := dec.Decode(&result); err != nil {
		return nil, fmt.Errorf("Failed to decode response XML: %v", err)
	}
	if result.Success == 0 {
		return nil, fmt.Errorf("Error retrieving thermostat status from %s: %s", resp.Request.URL, result.Message)
	}

	// Time zone retrieval logic
	targetTimezone := fmt.Sprintf("https://%s.officeclimatecontrol.net/api.cgi", sitename)
	respTimezone, _, errsTimezone := gorequest.New().Get(targetTimezone).
		Param("username", username).
		Param("password", password).
		Param("request", "get").
		Param("object", "Site").
		Param("value", "timeZone;").
		End()
	if errsTimezone != nil {
		return nil, fmt.Errorf("Error retrieving object result from %s: %s", targetTimezone, errsTimezone)
	}
	defer respTimezone.Body.Close()
	var resultTimezone apiResultSite
	decTimezone := xml.NewDecoder(respTimezone.Body)
	if err := decTimezone.Decode(&resultTimezone); err != nil {
		return nil, fmt.Errorf("Failed to decode response XML: %v", err)
	}

	timezone, timeErr := time.LoadLocation(resultTimezone.Attribute.Timezone)
	if timeErr != nil {
		return nil, fmt.Errorf("Invalid Timezone specified in pelican struct: %v\n", timeErr)
	}

	var pelicans []*Pelican
	for _, thermInfo := range result.Thermostats {
		if thermInfo.Name != "" {
			pelicans = append(pelicans, NewPelican(username, password, sitename, thermInfo.Name, timezone))
		}
	}
	return pelicans, nil
}

func (pel *Pelican) GetStatus() (*PelicanStatus, error) {
	resp, _, errs := pel.req.Get(pel.target).
		Param("username", pel.username).
		Param("password", pel.password).
		Param("request", "get").
		Param("object", "Thermostat").
		Param("selection", fmt.Sprintf("name:%s;", pel.name)).
		Param("value", "temperature;humidity;heatSetting;coolSetting;setBy;HeatNeedsFan;system;runStatus;statusDisplay").
		End()
	if errs != nil {
		return nil, fmt.Errorf("Error retrieving thermostat status from %s: %v", pel.target, errs)
	}

	defer resp.Body.Close()
	var result apiResult
	dec := xml.NewDecoder(resp.Body)
	if err := dec.Decode(&result); err != nil {
		return nil, fmt.Errorf("Failed to decode response XML: %v", err)
	}
	if result.Success == 0 {
		return nil, fmt.Errorf("Error retrieving thermostat status from %s: %s", resp.Request.URL, result.Message)
	}

	thermostat := result.Thermostat

	if thermostat.StatusDisplay == "Unreachable" {
		fmt.Printf("Thermostat %s is unreachable\n", pel.name)
		return nil, nil
	}

	var fanState bool
	if strings.HasPrefix(thermostat.RunStatus, "Heat") {
		fanState = thermostat.HeatNeedsFan == "Yes"
	} else if thermostat.RunStatus != "Off" {
		fanState = true
	} else {
		fanState = false
	}
	thermState, ok := stateMappings[thermostat.RunStatus]
	if !ok {
		// Thermostat is not calling for heating or cooling
		if thermostat.System == "Off" {
			thermState = 0 // Off
		} else {
			// Thermostat is not heating or cooling, but fan is still running
			// Report this as off
			thermState = 0 //Off
		}
	}

	// Thermostat History Object Request to retrieve time stamps from past hour
	endTime := time.Now().In(pel.timezone).Format(time.RFC3339)
	startTime := time.Now().Add(-1 * time.Hour).In(pel.timezone).Format(time.RFC3339)

	respHist, _, errsHist := pel.req.Get(pel.target).
		Param("username", pel.username).
		Param("password", pel.password).
		Param("request", "get").
		Param("object", "ThermostatHistory").
		Param("selection", fmt.Sprintf("startDateTime:%s;endDateTime:%s;", startTime, endTime)).
		Param("value", "timestamp").
		End()
	defer respHist.Body.Close()

	if errsHist != nil {
		return nil, fmt.Errorf("Error retrieving thermostat status from %s: %v", pel.target, errsHist)
	}

	var histResult apiResultHistory
	histDec := xml.NewDecoder(respHist.Body)
	if histErr := histDec.Decode(&histResult); histErr != nil {
		return nil, fmt.Errorf("Failed to decode response XML: %v", histErr)
	}
	if histResult.Success == 0 {
		return nil, fmt.Errorf("Error retrieving thermostat status from %s: %s", respHist.Request.URL, histResult.Message)
	}

	if len(histResult.Records.History) == 0 {
		return nil, nil
	}

	// Converting string timeStamp to int64 format
	match := histResult.Records.History[len(histResult.Records.History)-1]
	timestamp, timeErr := time.ParseInLocation("2006-01-02T15:04", match.TimeStamp, pel.timezone)
	if timeErr != nil {
		return nil, fmt.Errorf("Error parsing %v into Time struct: %v\n", match.TimeStamp, timeErr)
	}

	now := time.Now()
	if timestamp.Before(now.Add(-2 * time.Hour)) {
		fmt.Println("WARNING temperature data has not changed for 2 hours. This is not necessarily an error")
	}

	return &PelicanStatus{
		Temperature:     thermostat.Temperature,
		RelHumidity:     float64(thermostat.RelHumidity),
		HeatingSetpoint: float64(thermostat.HeatingSetpoint),
		CoolingSetpoint: float64(thermostat.CoolingSetpoint),
		Override:        thermostat.SetBy != "Schedule",
		Fan:             fanState,
		Mode:            modeNameMappings[thermostat.System],
		State:           thermState,
		Time:            now.UnixNano(),
	}, nil
}

func (pel *Pelican) ModifySetpoints(setpoints *setpointsMsg) error {
	var value string
	// heating setpoint
	if setpoints.HeatingSetpoint != nil {
		value += fmt.Sprintf("heatSetting:%d;", int(*setpoints.HeatingSetpoint))
	}
	// cooling setpoint
	if setpoints.CoolingSetpoint != nil {
		value += fmt.Sprintf("coolSetting:%d;", int(*setpoints.CoolingSetpoint))
	}
	resp, _, errs := pel.req.Get(pel.target).
		Param("username", pel.username).
		Param("password", pel.password).
		Param("request", "set").
		Param("object", "thermostat").
		Param("selection", fmt.Sprintf("name:%s;", pel.name)).
		Param("value", value).
		End()
	if errs != nil {
		return fmt.Errorf("Error modifying thermostat temp settings: %v", errs)
	}

	defer resp.Body.Close()
	var result apiResult
	dec := xml.NewDecoder(resp.Body)
	if err := dec.Decode(&result); err != nil {
		return fmt.Errorf("Failed to decode response XML: %v", err)
	}
	if result.Success == 0 {
		return fmt.Errorf("Error modifying thermostat temp settings: %v", result.Message)
	}

	return nil
}

func (pel *Pelican) ModifyState(params *pelicanStateParams) error {
	var value string

	// mode
	if params.Mode != nil {
		mode := int(*params.Mode)
		if mode < 0 || mode > 3 {
			return fmt.Errorf("Specified thermostat mode %d is invalid", mode)
		}
		systemVal := modeValMappings[mode]
		value += fmt.Sprintf("system:%s;", systemVal)
	}

	// override
	if params.Override != nil {
		var scheduleVal string
		if *params.Override == 1 {
			scheduleVal = "Off"
		} else {
			scheduleVal = "On"
		}
		value += fmt.Sprintf("schedule:%s;", scheduleVal)
	}

	// fan
	if params.Fan != nil {
		var fanVal string
		if *params.Fan == 1 {
			fanVal = "On"
		} else {
			fanVal = "Auto"
		}
		value += fmt.Sprintf("fan:%s;", fanVal)
	}

	// heating setpoint
	if params.HeatingSetpoint != nil {
		value += fmt.Sprintf("heatSetting:%d;", int(*params.HeatingSetpoint))
	}
	// cooling setpoint
	if params.CoolingSetpoint != nil {
		value += fmt.Sprintf("coolSetting:%d;", int(*params.CoolingSetpoint))
	}

	resp, _, errs := pel.req.Get(pel.target).
		Param("username", pel.username).
		Param("password", pel.password).
		Param("request", "set").
		Param("object", "thermostat").
		Param("selection", fmt.Sprintf("name:%s;", pel.name)).
		Param("value", value).
		End()
	if errs != nil {
		return fmt.Errorf("Error modifying thermostat state: %v (%s)", errs, resp.Request.URL)
	}

	defer resp.Body.Close()
	var result apiResult
	dec := xml.NewDecoder(resp.Body)
	if err := dec.Decode(&result); err != nil {
		return fmt.Errorf("Failed to decode response XML: %v", err)
	}
	if result.Success == 0 {
		return fmt.Errorf("Error modifying thermostat state: %s", result.Message)
	}

	return nil
}
