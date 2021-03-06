package main

import (
	"encoding/xml"
	"strconv"

	"github.com/pkg/errors"
)

type HexInt64 int64

func (v *HexInt64) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var s string
	if err := d.DecodeElement(&s, &start); err != nil {
		log.Error(err)
		return err
	}
	// skip the "0x" prefix
	if len(s) < 3 {
		return errors.Errorf("Invalid string: %s", s)
	}
	val, err := strconv.ParseInt(s[2:], 16, 64)
	*v = HexInt64(val)
	return err
}

func (v *HexInt64) Int64() int64 {
	return int64(*v)
}
