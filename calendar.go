package calendar

import (
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	Weekend    = 1 // Выходной день
	ShortDay   = 2 // Короткий день
	WorkingDay = 3 // рабочий день (суббота/воскресенье)

	Country_by    = "by"    // Календарь Республики Беларусь
	Country_kz    = "kz"    // Календарь Республики Казахстан
	Country_ru    = "ru"    // Календарь России (на русском языке)
	Country_ru_en = "ru:en" // Календарь России (на английском языке)
	Country_ua    = "ua"    // Календарь Украины
	Country_uz    = "uz"    // Календарь Узбекистана
)

// Производственный календарь
type ProductionCalendar struct {
	sync.Mutex

	country       string        // код страны
	lang          string        // код языка
	cacheDuration time.Duration // продолжительность кэширования данных

	yearMap map[int]*calendarYear // хранит данные производственных календарей по годам
}

// Распарсенный производственный календарь заданного года
type calendarYear struct {
	dayMap     map[int64]calendarDay // карта с особыми днями календаря, ключ Y.m.d
	weekendMap map[int64]int64       // карта переноса выходных дней
	holidays   map[int]string        // карта названий праздников
	expiresAt  int64                 // timestamp когда истекает кэш
}

// особый день календаря
type calendarDay struct {
	T int   // тип дня (см. константы Weekend, ShortDay, WorkingDay)
	H int   // идентификатор праздника
	F int64 // timestamp даты, с которой был перенесен выходной день
}

// New возвращает указатель на новый объект календаря.
//
// country - страна
//
// cacheDuration - время кэширования данных календаря в секундах.
// .
func New(country string, cacheDuration time.Duration) *ProductionCalendar {

	lang := country
	parts := strings.Split(country, ":")

	if len(parts) > 1 {
		country = parts[0]
		lang = parts[1]
	}

	return &ProductionCalendar{
		country:       country,
		lang:          lang,
		cacheDuration: cacheDuration,
		yearMap:       make(map[int]*calendarYear),
	}
}

// GetCountry возвращает страну календаря.
func (cal *ProductionCalendar) GetCountry() string {
	return cal.country
}

// GetLocale возвращает язык производственного календаря.
func (cal *ProductionCalendar) GetLang() string {
	return cal.lang
}

// GetCacheDurtion возвращает время хранения кэша в секундах.
func (cal *ProductionCalendar) GetCacheDurtion() time.Duration {
	return cal.cacheDuration
}

// GetSourcePublicUrl возвращает ссылку-источик, где опубликованы файлы с календарями.
func (cal *ProductionCalendar) GetSourcePublicUrl() string {
	return "https://raw.githubusercontent.com/xmlcalendar/data/refs/heads/master"
}

// GetCalendarXml - возвращает ссылку на xml-файл производственного календаря.
// Который расположен в публичном репозитории сайта xmlcalendar.ru, с учетом страны и языка.
//
// year - год календаря, который надо получить.
// .
func (cal *ProductionCalendar) GetCalendarXml(year int) string {

	if cal.country == cal.lang {
		return fmt.Sprintf("%s/%s/%d/calendar.xml", cal.GetSourcePublicUrl(), cal.country, year)
	}

	return fmt.Sprintf("%s/%s/%d/calendar.%s.xml", cal.GetSourcePublicUrl(), cal.country, year, cal.lang)
}

// getXmlContent выполняет чтение xml-файла для получения контента.
func (cal *ProductionCalendar) getXmlContent(url string) (string, error) {

	client := &http.Client{Timeout: 30 * time.Second} // TODO:: timeout to opts

	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	return string(body), nil
}

// loadCalendarData выполняет загрузку данных за заданный год.
func (cal *ProductionCalendar) loadXmlCalendar(year int) (*xmlcalendar, error) {

	xmlUrl := cal.GetCalendarXml(year)
	xmlContent, err := cal.getXmlContent(xmlUrl)

	if err != nil {
		return nil, fmt.Errorf("failed to fetch XML content: %w", err)
	}

	xmlcal, err := parseCalendarXML(xmlContent)

	if err != nil {
		return nil, fmt.Errorf("failed to parse XML content: %w", err)
	}

	return xmlcal, nil
}

// prepareCalendarYear - выполняет подготовку данных, полученных из xmlcalendar.
func (cal *ProductionCalendar) prepareCalendarYear(xmlcal *xmlcalendar) (*calendarYear, error) {

	calYear := calendarYear{
		dayMap:     make(map[int64]calendarDay),
		weekendMap: make(map[int64]int64),
		holidays:   make(map[int]string),
		expiresAt:  time.Now().Unix() + int64(cal.cacheDuration.Seconds()),
	}

	for _, holiday := range xmlcal.Holidays {
		calYear.holidays[holiday.ID] = holiday.Title
	}

	for _, xmlday := range xmlcal.Days {
		var fts int64
		date := fmt.Sprintf("%d.%s", xmlcal.Year, xmlday.D)
		timeD, err := time.Parse("2006.01.02", date)

		if err != nil {
			return nil, fmt.Errorf("failed to parse date '%s': %w", date, err)
		}

		if xmlday.F != "" {
			weekendTo := fmt.Sprintf("%d.%s", xmlcal.Year, xmlday.F)

			timeF, err := time.Parse("2006.01.02", weekendTo)

			if err != nil {
				return nil, fmt.Errorf("failed to parse date'%s': %w", weekendTo, err)
			}

			fts = timeF.Unix()
			calYear.weekendMap[fts] = timeD.Unix()
		}

		calYear.dayMap[timeD.Unix()] = calendarDay{
			T: xmlday.T,
			F: fts,
			H: xmlday.H,
		}
	}

	return &calYear, nil
}

// getCalendarYear возвращает подготовленную структуру производственного календаря за год.
func (cal *ProductionCalendar) getCalendarYear(year int) (*calendarYear, error) {

	cal.Lock()
	calYear, ok := cal.yearMap[year]
	cal.Unlock()

	now := time.Now().Unix()

	if !ok || (ok && now > calYear.expiresAt) {
		xmlcalendar, err := cal.loadXmlCalendar(year)

		if err != nil {
			return nil, fmt.Errorf("failed to load calendar year: %w", err)
		}

		loadedYear, err := cal.prepareCalendarYear(xmlcalendar)

		if err != nil {
			return nil, fmt.Errorf("failed to prepare calendar year: %w", err)
		}

		cal.Lock()
		cal.yearMap[year] = loadedYear
		cal.Unlock()
		calYear = loadedYear
	}

	return calYear, nil
}

// CheckFullWorkingDay - возращает true, если date ПОЛНЫЙ РАБОЧИЙ день.
// Если день рабочий, но короткий, метод вернет FALSE.
//
// Для простой проверки рабочего дня (рабочий/не рабочий) используйте метод CheckWorkingDay.
// .
func (cal *ProductionCalendar) CheckFullWorkingDay(date time.Time, weekends []int) (bool, error) {

	ts := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location()).Unix()
	calYear, err := cal.getCalendarYear(date.Year())

	if err != nil {
		return false, fmt.Errorf("failed to get year calendar: %w", err)
	}

	day, ok := calYear.dayMap[ts]

	if ok {
		return (day.T == WorkingDay), nil
	}

	return !slices.Contains(weekends, int(date.Weekday())), nil
}

// CheckHoliday - возвращает true, если date праздник.
// Праздник это всегда выходной день.
func (cal *ProductionCalendar) CheckHoliday(date time.Time) (bool, error) {

	ts := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location()).Unix()
	calYear, err := cal.getCalendarYear(date.Year())

	if err != nil {
		return false, fmt.Errorf("failed to get year calendar: %w", err)
	}

	day, ok := calYear.dayMap[ts]

	if ok {
		return (day.T == Weekend && day.H > 0), nil
	}

	return false, nil
}

// GetHolidayName - возвращает название праздника по дате, или пустую строку, если праздника нет.
func (cal *ProductionCalendar) GetHolidayName(date time.Time) (string, error) {

	ts := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location()).Unix()
	calYear, err := cal.getCalendarYear(date.Year())

	if err != nil {
		return "", fmt.Errorf("failed to get year calendar: %w", err)
	}

	day, ok := calYear.dayMap[ts]

	if ok {

		holidayName, ok := calYear.holidays[day.H]

		if ok {
			return holidayName, nil
		}
	}

	return "", nil
}

// CheckShortWorkingDay - возращает true, если date предпраздничный (короткий) РАБОЧИЙ день.
func (cal *ProductionCalendar) CheckShortWorkingDay(date time.Time) (bool, error) {

	ts := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location()).Unix()
	calYear, err := cal.getCalendarYear(date.Year())

	if err != nil {
		return false, fmt.Errorf("failed to get year calendar: %w", err)
	}

	day, ok := calYear.dayMap[ts]

	if ok {
		return (day.T == ShortDay), nil
	}

	return false, nil
}

// checkWorkingDay - возвращает true, если date РАБОЧИЙ день (ПОЛНЫЙ или КОРОТКИЙ).
func (cal *ProductionCalendar) CheckWorkingDay(date time.Time, weekends []int) (bool, error) {

	checkFull, err := cal.CheckFullWorkingDay(date, weekends)

	if err != nil {
		return false, fmt.Errorf("failed to get year calendar: %w", err)
	}

	checkShort, err := cal.CheckShortWorkingDay(date)

	if err != nil {
		return false, fmt.Errorf("failed to get year calendar: %w", err)
	}

	return (checkFull || checkShort), nil
}

// CheckWeekend - Возращает true, если date выходной день.
// Выходным днем считаются дни недели, определенные в weekends или праздник.
func (cal *ProductionCalendar) CheckWeekend(date time.Time, weekends []int) (bool, error) {

	ts := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location()).Unix()
	calYear, err := cal.getCalendarYear(date.Year())

	if err != nil {
		return false, fmt.Errorf("failed to get year calendar: %w", err)
	}

	day, ok := calYear.dayMap[ts]

	if ok {
		return (day.T == Weekend), nil
	}

	return slices.Contains(weekends, int(date.Weekday())), nil
}

// GetWeekendFrom - Возвращает дату, с которой осуществляется перенос выходного дня на дату date.
// В общем случае дата date становится выходным днем, а возвращаемая дата - рабочим,
// за исключением, когда перенос осуществляется с выходного дня праздника.
// Если переноса нет, то метод вернёт пустую структуру time.Time{}, у которой метод IsZero() будет возвращать true.
func (cal *ProductionCalendar) GetWeekendFrom(date time.Time) (time.Time, error) {

	ts := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location()).Unix()
	calYear, err := cal.getCalendarYear(date.Year())

	if err != nil {
		return time.Time{}, fmt.Errorf("failed to get year calendar: %w", err)
	}

	day, ok := calYear.dayMap[ts]

	if ok && day.F > 0 {
		return time.Unix(day.F, 0), nil
	}

	return time.Time{}, nil
}

// GetWeekendTo - возвращает дату, на которую осуществляется перенос выходного дня с даты date.
// В общем случае дата date становится рабочим днем, а возвращаемая дата - выходным,
// за исключением, когда перенос осуществляется с выходного дня праздника.
func (cal *ProductionCalendar) GetWeekendTo(date time.Time) (time.Time, error) {

	ts := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location()).Unix()
	calYear, err := cal.getCalendarYear(date.Year())

	if err != nil {
		return time.Time{}, fmt.Errorf("failed to get year calendar: %w", err)
	}

	weekendTs, ok := calYear.weekendMap[ts]

	if ok {

		return time.Unix(weekendTs, 0), nil
	}

	return time.Time{}, nil
}

// GetNextWorkingDay - возвращает дату следующего рабочего дня после date.
func (cal *ProductionCalendar) GetNextWorkingDay(date time.Time, weekends []int) (time.Time, error) {

	for {
		date = date.AddDate(0, 0, 1)
		ok, err := cal.CheckWorkingDay(date, weekends)

		if err != nil {

			return time.Time{}, fmt.Errorf("failed to get next working day: %w", err)
		}

		if ok {
			break
		}
	}

	return date, nil
}

// GetWeekendDates - возвращает срез time.Time последовательных выходных дней, в который входит date.
// Если date не выходной день, то метод вернет пустой срез.
//
// date - дата, относительно которой проверяется следующий рабочий день.
//
// weekends срез с номерами дней недели, которые являются выходными.
//
// full - если true, то срез будет включать все даты, в том числ предшествующие и равную date, иначе только даты после date
// .
func (cal *ProductionCalendar) GetWeekendDates(date time.Time, weekends []int, full bool) ([]time.Time, error) {

	ok, err := cal.CheckWeekend(date, weekends)

	if err != nil {
		return nil, fmt.Errorf("failed to get weekend dates: %w", err)
	}

	if ok {
		if full {

			for {
				ok, err = cal.CheckWeekend(date, weekends)

				if err != nil {
					return nil, fmt.Errorf("failed to get weekend dates: %w", err)
				}

				if !ok {
					break
				}
				date = date.AddDate(0, 0, -1)
			}
		}

		weekendDates := []time.Time{}

		for {
			date = date.AddDate(0, 0, 1)

			ok, err = cal.CheckWeekend(date, weekends)

			if err != nil {
				return nil, fmt.Errorf("failed to get weekend dates: %w", err)
			}

			if !ok {
				break
			}
			weekendDates = append(weekendDates, date)
		}

		return weekendDates, nil
	}

	return nil, nil
}

// GetHolidayDates - возвращает срез дат последовательных праздников, в который входит date.
// Если date не праздник, то метод вернет пустой срез (nil), даже если date выходной день.
//
// date - дата относительно которой проверяем следующий рабочий день.
//
// full - если true, то срез будет включать все даты, в том числе предшествующие и равную date, иначе только даты после date
// .
func (cal *ProductionCalendar) GetHolidayDates(date time.Time, full bool) ([]time.Time, error) {

	ok, err := cal.CheckHoliday(date)

	if err != nil {
		return nil, fmt.Errorf("failed to get holiday dates: %w", err)
	}

	if ok {
		if full {

			for {
				ok, err = cal.CheckHoliday(date)

				if err != nil {
					return nil, fmt.Errorf("failed to get holiday dates: %w", err)
				}

				if !ok {
					break
				}
				date = date.AddDate(0, 0, -1)
			}
		}

		holidayDates := []time.Time{}

		for {
			date = date.AddDate(0, 0, 1)

			ok, err = cal.CheckHoliday(date)

			if err != nil {
				return nil, fmt.Errorf("failed to get holiday dates: %w", err)
			}

			if !ok {
				break
			}
			holidayDates = append(holidayDates, date)
		}

		return holidayDates, nil
	}

	return nil, nil
}
