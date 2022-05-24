/*
 Copyright © 2022 Dell Inc. or its subsidiaries. All Rights Reserved.

 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at
      http://www.apache.org/licenses/LICENSE-2.0
 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package display

import (
	"fmt"
	"io"
	"reflect"
	"text/tabwriter"
)

// TableWriter is a structure that allows to print resources in table form
type TableWriter struct {
	writer  *tabwriter.Writer
	objName string
	header  string
	format  string
}

func getHeaderAndFormatStrings(obj interface{}) (string, string) {
	v := reflect.TypeOf(obj)
	var format, header string
	for i := 0; i < v.NumField(); i++ {
		typeField := v.Field(i)
		tag := v.Field(i).Tag
		displayKey := ""
		tmpHeader := ""
		if tag != "" {
			displayKey = tag.Get("display")
			if displayKey != "" {
				tmpHeader += displayKey + "\t"
			} else {
				continue
			}
		} else {
			continue
		}
		tmpFormat := getFormatString(typeField.Type.Kind())
		if tmpFormat != "" {
			format += tmpFormat + "\t"
			header += tmpHeader
		}
	}
	if format != "" {
		format += "\n"
	}
	return header, format
}

func getFormatString(kind reflect.Kind) string {
	format := ""
	switch kind {
	case reflect.Bool:
		fallthrough
	case reflect.String:
		fallthrough
	case reflect.Int:
		fallthrough
	case reflect.Float32:
		fallthrough
	case reflect.Float64:
		format += "%v"
	default:
		format += "%+v"
	}
	return format
}

// NewTableWriter initializes and returns new instance of TableWriter
func NewTableWriter(obj interface{}, out io.Writer) (*TableWriter, error) {
	w := tabwriter.NewWriter(out, 0, 8, 1, '\t', 0)
	header, format := getHeaderAndFormatStrings(obj)
	objName := reflect.TypeOf(obj).Name()
	if header == "" || format == "" {
		return nil, fmt.Errorf("unable to determine headers")
	}
	return &TableWriter{
		writer:  w,
		header:  header,
		format:  format,
		objName: objName,
	}, nil
}

func header(length int) string {
	header := "+"
	for i := 0; i < length; i++ {
		header += "-"
	}
	header += "+"
	return header
}

// PrintHeader prints header of the table
func (t *TableWriter) PrintHeader() {
	length := len(t.objName)
	paddingLength := 2 * length
	header := header(paddingLength + 1)
	formatString := fmt.Sprintf("| %%-%dv|\n", paddingLength)
	_, _ = fmt.Fprintln(t.writer, header)
	_, _ = fmt.Fprintf(t.writer, formatString, t.objName)
	_, _ = fmt.Fprintln(t.writer, header)
	_, _ = fmt.Fprintln(t.writer, t.header)
}

// PrintRow prints rows of the tables
func (t *TableWriter) PrintRow(obj interface{}) {
	var values []interface{}
	v := reflect.ValueOf(obj)
	for i := 0; i < v.NumField(); i++ {
		valueField := v.Field(i)
		tag := v.Type().Field(i).Tag
		if tag != "" {
			if tag.Get("display") != "" {
				values = append(values, valueField.Interface())
			} else {
				continue
			}
		} else {
			continue
		}
	}
	_, _ = fmt.Fprintf(t.writer, t.format, values...)
}

// Done flushes table writer
func (t *TableWriter) Done() {
	_ = t.writer.Flush()
}
