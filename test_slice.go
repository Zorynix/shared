package main

import (
	"fmt"
	"os"
	"reflect"
	"strings"
)

type TestConfig struct {
	Brokers []string `yaml:"brokers"`
}

func setFieldFromString(field reflect.Value, value string) error {
	switch field.Kind() {
	case reflect.Slice:
		if field.Type().Elem().Kind() == reflect.String {
			values := strings.Split(value, ",")
			slice := reflect.MakeSlice(field.Type(), len(values), len(values))
			for i, v := range values {
				slice.Index(i).SetString(strings.TrimSpace(v))
			}
			field.Set(slice)
		} else {
			return fmt.Errorf("unsupported slice element type: %s", field.Type().Elem().Kind())
		}
	default:
		return fmt.Errorf("unsupported field type: %s", field.Kind())
	}
	return nil
}

func main() {
	config := &TestConfig{}
	v := reflect.ValueOf(config).Elem()
	field := v.Field(0)

	err := setFieldFromString(field, "broker1:9092,broker2:9092,broker3:9092")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Success! Brokers: %+v\n", config.Brokers)
}
