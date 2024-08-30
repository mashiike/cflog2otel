package cflog2otel

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/samber/oops"
)

type CELVariablesLog struct {
	Type                   string    `json:"type" cel:"type"`
	Date                   string    `json:"date" cel:"date"`
	Time                   string    `json:"time" cel:"time"`
	Timestamp              time.Time `json:"timestamp" cel:"timestamp"`
	EdgeLocation           *string   `json:"edgeLocation" cel:"edgeLocation"`
	ScBytes                *int      `json:"scBytes" cel:"scBytes"`
	ClientIP               *string   `json:"clientIp" cel:"clientIp"`
	CsMethod               *string   `json:"csMethod" cel:"csMethod"`
	CsHost                 *string   `json:"csHost" cel:"csHost"`
	CsURIStem              *string   `json:"csUriStem" cel:"csUriStem"`
	ScStatus               *int      `json:"scStatus" cel:"scStatus"`
	ScStatusCategory       *string   `json:"scStatusCategory" cel:"scStatusCategory"`
	CsReferer              *string   `json:"csReferer" cel:"csReferer"`
	CsUserAgent            *string   `json:"csUserAgent" cel:"csUserAgent"`
	CsURIQuery             *string   `json:"csUriQuery" cel:"csUriQuery"`
	CsCookie               *string   `json:"csCookie" cel:"csCookie"`
	EdgeResultType         *string   `json:"edgeResultType" cel:"edgeResultType"`
	EdgeRequestID          *string   `json:"edgeRequestId" cel:"edgeRequestId"`
	HostHeader             *string   `json:"hostHeader" cel:"hostHeader"`
	CsProtocol             *string   `json:"csProtocol" cel:"csProtocol"`
	CsBytes                *int      `json:"csBytes" cel:"csBytes"`
	TimeTaken              *float64  `json:"timeTaken" cel:"timeTaken"`
	XForwardedFor          *string   `json:"xForwardedFor" cel:"xForwardedFor"`
	SslProtocol            *string   `json:"sslProtocol" cel:"sslProtocol"`
	SslCipher              *string   `json:"sslCipher" cel:"sslCipher"`
	EdgeResponseResultType *string   `json:"edgeResponseResultType" cel:"edgeResponseResultType"`
	CsProtocolVersion      *string   `json:"csProtocolVersion" cel:"csProtocolVersion"`
	FleStatus              *string   `json:"fleStatus" cel:"fleStatus"`
	FleEncryptedFields     *int      `json:"fleEncryptedFields" cel:"fleEncryptedFields"`
	CPort                  *int      `json:"cPort" cel:"cPort"`
	TimeToFirstByte        *float64  `json:"timeToFirstByte" cel:"timeToFirstByte"`
	EdgeDetailedResultType *string   `json:"edgeDetailedResultType" cel:"edgeDetailedResultType"`
	ScContentType          *string   `json:"scContentType" cel:"scContentType"`
	ScContentLen           *int      `json:"scContentLen" cel:"scContentLen"`
	ScRangeStart           *string   `json:"scRangeStart" cel:"scRangeStart"`
	ScRangeEnd             *string   `json:"scRangeEnd" cel:"scRangeEnd"`
}

func (l *CELVariablesLog) CloudFrontStandardLogFieldSetters() map[string]func(string) error {
	//#Fields: date time x-edge-location sc-bytes c-ip cs-method cs(Host) cs-uri-stem sc-status cs(Referer) cs(User-Agent) cs-uri-query cs(Cookie) x-edge-result-type x-edge-request-id x-host-header cs-protocol cs-bytes time-taken x-forwarded-for ssl-protocol ssl-cipher x-edge-response-result-type cs-protocol-version fle-status fle-encrypted-fields c-port time-to-first-byte x-edge-detailed-result-type sc-content-type sc-content-len sc-range-start sc-range-end
	setters := make(map[string]func(string) error, 36)
	setters["date"] = func(s string) error {
		l.Date = s
		if l.Time != "" {
			t, err := time.Parse("2006-01-02 15:04:05", l.Date+" "+l.Time)
			if err != nil {
				return oops.Wrapf(err, "failed to parse date and time")
			}
			l.Timestamp = t
		}
		return nil
	}
	setters["time"] = func(s string) error {
		l.Time = s
		if l.Date != "" {
			t, err := time.Parse("2006-01-02 15:04:05", l.Date+" "+l.Time)
			if err != nil {
				return oops.Wrapf(err, "failed to parse date and time")
			}
			l.Timestamp = t
		}
		return nil
	}
	setters["x-edge-location"] = func(s string) error {
		l.EdgeLocation = toPtrString(s)
		return nil
	}
	setters["sc-bytes"] = func(s string) error {
		val, err := toPtrInt(s)
		if err != nil {
			return err
		}
		l.ScBytes = val
		return nil
	}
	setters["c-ip"] = func(s string) error {
		l.ClientIP = toPtrString(s)
		return nil
	}
	setters["cs-method"] = func(s string) error {
		l.CsMethod = toPtrString(s)
		return nil
	}
	setters["cs(Host)"] = func(s string) error {
		l.CsHost = toPtrString(s)
		return nil
	}
	setters["cs-uri-stem"] = func(s string) error {
		l.CsURIStem = toPtrString(s)
		return nil
	}
	setters["sc-status"] = func(s string) error {
		val, err := toPtrInt(s)
		if err != nil {
			return oops.Errorf("failed to convert sc-status")
		}
		l.ScStatus = val
		if val != nil {
			l.ScStatusCategory = toPtrString(fmt.Sprintf("%dxx", *val/100))
		}
		return nil
	}
	setters["cs(Referer)"] = func(s string) error {
		l.CsReferer = toPtrString(s)
		return nil
	}
	setters["cs(User-Agent)"] = func(s string) error {
		if s == "-" {
			return nil
		}
		unescaped, err := url.QueryUnescape(s)
		if err != nil {
			return oops.Errorf("failed to unescape cs(User-Agent)")
		}
		l.CsUserAgent = &unescaped
		return nil
	}
	setters["cs-uri-query"] = func(s string) error {
		l.CsURIQuery = toPtrString(s)
		return nil
	}
	setters["cs(Cookie)"] = func(s string) error {
		l.CsCookie = toPtrString(s)
		return nil
	}
	setters["x-edge-result-type"] = func(s string) error {
		l.EdgeResultType = toPtrString(s)
		return nil
	}
	setters["x-edge-request-id"] = func(s string) error {
		l.EdgeRequestID = toPtrString(s)
		return nil
	}
	setters["x-host-header"] = func(s string) error {
		l.HostHeader = toPtrString(s)
		return nil
	}
	setters["cs-protocol"] = func(s string) error {
		l.CsProtocol = toPtrString(s)
		return nil
	}
	setters["cs-bytes"] = func(s string) error {
		val, err := toPtrInt(s)
		if err != nil {
			return oops.Errorf("failed to convert cs-bytes")
		}
		l.CsBytes = val
		return nil
	}
	setters["time-taken"] = func(s string) error {
		val, err := toPtrFloat64(s)
		if err != nil {
			return oops.Errorf("failed to convert time-taken")
		}
		l.TimeTaken = val
		return nil
	}
	setters["x-forwarded-for"] = func(s string) error {
		l.XForwardedFor = toPtrString(s)
		return nil
	}
	setters["ssl-protocol"] = func(s string) error {
		l.SslProtocol = toPtrString(s)
		return nil
	}
	setters["ssl-cipher"] = func(s string) error {
		l.SslCipher = toPtrString(s)
		return nil
	}
	setters["x-edge-response-result-type"] = func(s string) error {
		l.EdgeResponseResultType = toPtrString(s)
		return nil
	}
	setters["cs-protocol-version"] = func(s string) error {
		l.CsProtocolVersion = toPtrString(s)
		return nil
	}
	setters["fle-status"] = func(s string) error {
		l.FleStatus = toPtrString(s)
		return nil
	}
	setters["fle-encrypted-fields"] = func(s string) error {
		val, err := toPtrInt(s)
		if err != nil {
			return oops.Errorf("failed to convert fle-encrypted-fields")
		}
		l.FleEncryptedFields = val
		return nil
	}
	setters["c-port"] = func(s string) error {
		val, err := toPtrInt(s)
		if err != nil {
			return oops.Errorf("failed to convert c-port")
		}
		l.CPort = val
		return nil
	}
	setters["time-to-first-byte"] = func(s string) error {
		val, err := toPtrFloat64(s)
		if err != nil {
			return oops.Errorf("failed to convert time-to-first-byte")
		}
		l.TimeToFirstByte = val
		return nil
	}
	setters["x-edge-detailed-result-type"] = func(s string) error {
		l.EdgeDetailedResultType = toPtrString(s)
		return nil
	}
	setters["sc-content-type"] = func(s string) error {
		l.ScContentType = toPtrString(s)
		return nil
	}
	setters["sc-content-len"] = func(s string) error {
		val, err := toPtrInt(s)
		if err != nil {
			return oops.Errorf("failed to convert sc-content-len")
		}
		l.ScContentLen = val
		return nil
	}
	setters["sc-range-start"] = func(s string) error {
		l.ScRangeStart = toPtrString(s)
		return nil
	}
	setters["sc-range-end"] = func(s string) error {
		l.ScRangeEnd = toPtrString(s)
		return nil
	}
	return setters
}

func toPtrString(s string) *string {
	if s == "-" {
		return nil
	}
	return &s
}

func toPtrInt(s string) (*int, error) {
	if s == "-" {
		return nil, nil
	}
	val, err := strconv.Atoi(s)
	if err != nil {
		return nil, err
	}
	return &val, nil
}

func toPtrFloat64(s string) (*float64, error) {
	if s == "-" {
		return nil, nil
	}
	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil, err
	}
	return &val, nil
}

func ParseCloudFrontLog(ctx context.Context, r io.Reader) ([]CELVariablesLog, error) {
	scanner := bufio.NewScanner(r)
	var fields []string
	logs := make([]CELVariablesLog, 0)
	lineCount := 0
	for scanner.Scan() {
		lineCount++
		line := scanner.Text()
		if strings.HasPrefix(line, "#") {
			part := strings.SplitN(line[1:], ":", 2)
			if len(part) != 2 {
				slog.DebugContext(ctx, "invalid header line", "line", line)
				continue
			}
			key := strings.TrimSpace(part[0])
			value := strings.TrimSpace(part[1])
			switch key {
			case "Version":
				slog.DebugContext(ctx, "cloud front log version", "value", value)
			case "Fields":
				fields = strings.Split(value, " ")
				slog.DebugContext(ctx, "cloud front log fields", "fields_count", len(fields))
			}
			continue
		}
		values := strings.Split(line, "\t")
		if len(values) > len(fields) {
			return nil, oops.Errorf("this row has more values then fields, num of values = %d, num of feilds = %d", len(values), len(fields))
		}
		l := CELVariablesLog{
			Type: "CloudFront Standard Log",
		}
		setters := l.CloudFrontStandardLogFieldSetters()
		for i, value := range values {
			if i >= len(fields) {
				continue
			}
			if setter, ok := setters[fields[i]]; ok {
				err := setter(value)
				if err != nil {
					return nil, oops.Wrapf(err, "failed to set field value[line=%d, field=%q]", lineCount, fields[i])
				}
				continue
			}
			slog.WarnContext(ctx, "unknown field detected", "field", fields[i], "line", lineCount)
		}
		logs = append(logs, l)
	}
	if err := scanner.Err(); err != nil {
		return nil, oops.Wrapf(err, "failed to scan log")
	}
	return logs, nil
}
