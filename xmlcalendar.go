package calendar

import (
	"encoding/xml"
	"fmt"
)

type xmlcalendar struct {
	XMLName  xml.Name  `xml:"calendar"`
	Year     int       `xml:"year,attr"`
	Lang     string    `xml:"lang,attr"`
	Country  string    `xml:"country,attr"`
	Holidays []holiday `xml:"holidays>holiday"`
	Days     []day     `xml:"days>day"`
}

type holiday struct {
	ID    int    `xml:"id,attr"`
	Title string `xml:"title,attr"`
}

type day struct {
	D string `xml:"d,attr"` // дата в формате mm.dd
	T int    `xml:"t,attr"` // тип дня (1 - нерабочий день, 2 - рабочий сокращенный день, рабочий день суббота или воскресенье)
	H int    `xml:"h,attr"` // ID связанного праздника (может отсутствовать)
	F string `xml:"f,attr"` // дата, с которой был перенесен выходной день в формате mm.dd
}

// parseCalendarXML принимает XML-строку и возвращает разобранную структуру
func parseCalendarXML(xmlStr string) (*xmlcalendar, error) {
	var cal xmlcalendar
	if err := xml.Unmarshal([]byte(xmlStr), &cal); err != nil {
		return nil, fmt.Errorf("failed to parse calendar xml: %w", err)
	}
	return &cal, nil
}
