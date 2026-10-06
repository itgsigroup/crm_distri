package importer

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
)

// ParseCSV reads a CSV export (comma or semicolon, UTF-8 with or without BOM). Header names are matched to the
// contract case-insensitively; unknown columns are ignored.
func ParseCSV(r io.Reader) ([]Row, error) {
	br := bufio.NewReader(r)
	head, err := br.Peek(4096)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, bufio.ErrBufferFull) {
		return nil, err
	}
	head = bytes.TrimPrefix(head, []byte("\xef\xbb\xbf"))
	first := string(head)
	if i := strings.IndexByte(first, '\n'); i >= 0 {
		first = first[:i]
	}
	if bom, _ := br.Peek(3); bytes.Equal(bom, []byte("\xef\xbb\xbf")) {
		_, _ = br.Discard(3)
	}
	cr := csv.NewReader(br)
	if strings.Count(first, ";") > strings.Count(first, ",") {
		cr.Comma = ';'
	}
	cr.FieldsPerRecord = -1
	cr.LazyQuotes = true
	header, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("header CSV: %w", err)
	}
	for i := range header {
		header[i] = strings.ToLower(strings.TrimSpace(header[i]))
	}
	var rows []Row
	for line := 2; ; line++ {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("baris %d: %w", line, err)
		}
		r := Row{}
		empty := true
		for i, v := range rec {
			if i < len(header) && header[i] != "" {
				r[header[i]] = v
				if strings.TrimSpace(v) != "" {
					empty = false
				}
			}
		}
		if !empty {
			rows = append(rows, r)
		}
	}
	return rows, nil
}

// Template is the CSV header of an entity.
func Template(e Entity) string {
	cols := make([]string, 0, len(Contract[e]))
	for _, c := range Contract[e] {
		cols = append(cols, c.Name)
	}
	return strings.Join(cols, ",") + "\n"
}
