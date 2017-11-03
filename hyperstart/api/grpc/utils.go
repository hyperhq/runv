package grpc

import (
	"fmt"
	"reflect"

	"github.com/opencontainers/runtime-spec/specs-go"
)

func copyValue(to, from reflect.Value) error {
	toKind := to.Kind()
	fromKind := from.Kind()

	if !from.IsValid() {
		return nil
	}

	if toKind == reflect.Ptr {
		// If the destination is a pointer, we need to allocate a new one.
		to.Set(reflect.New(to.Type().Elem()))
		if fromKind == reflect.Ptr {
			return copyValue(to.Elem(), from.Elem())
		} else {
			return copyValue(to.Elem(), from)
		}
	} else {
		// Here the destination is not a pointer.
		// Let's check what's the origin.
		if fromKind == reflect.Ptr {
			return copyValue(to, from.Elem())
		}

		switch toKind {
		case reflect.Struct:
			return copyStructValue(to, from)
		case reflect.Slice:
			return copySliceValue(to, from)
		case reflect.Map:
			return copyMapValue(to, from)
		default:
			// We now are copying non pointers scalar.
			// This is the leaf of the recursion.
			if from.Type() != to.Type() {
				if from.Type().ConvertibleTo(to.Type()) {
					to.Set(from.Convert(to.Type()))
					return nil
				} else {
					return fmt.Errorf("Can not convert %v to %v", from.Type(), to.Type())
				}
			} else {
				to.Set(from)
				return nil
			}
		}
	}
}

func copyMapValue(to, from reflect.Value) error {
	if to.Kind() != reflect.Map && from.Kind() != reflect.Map {
		return fmt.Errorf("Can only copy maps into maps")
	}

	to.Set(reflect.MakeMap(to.Type()))

	keys := from.MapKeys()

	for _, k := range keys {
		newValue := reflect.New(to.Type().Elem())
		v := from.MapIndex(k)

		if err := copyValue(newValue.Elem(), v); err != nil {
			return err
		}

		to.SetMapIndex(k, newValue.Elem())
	}

	return nil
}

func copySliceValue(to, from reflect.Value) error {
	if to.Kind() != reflect.Slice && from.Kind() != reflect.Slice {
		return fmt.Errorf("Can only copy slices into slices")
	}

	sliceLen := from.Len()
	to.Set(reflect.MakeSlice(to.Type(), sliceLen, sliceLen))

	for j := 0; j < sliceLen; j++ {
		if err := copyValue(to.Index(j), from.Index(j)); err != nil {
			return err
		}
	}

	return nil
}

func copyStructSkipField(to, from reflect.Value) bool {
	var grpcSolaris Solaris
	var ociSolaris specs.Solaris
	var grpcWindows Windows
	var ociWindows specs.Windows

	toType := to.Type()
	grpcSolarisType := reflect.TypeOf(grpcSolaris)
	ociSolarisType := reflect.TypeOf(ociSolaris)
	grpcWindowsType := reflect.TypeOf(grpcWindows)
	ociWindowsType := reflect.TypeOf(ociWindows)

	// We skip all Windows and Solaris types
	if toType == grpcSolarisType || toType == grpcWindowsType || toType == ociSolarisType || toType == ociWindowsType {
		return true
	}

	return false
}

func structFieldName(v reflect.Value, index int) (string, error) {
	if v.Kind() != reflect.Struct {
		return "", fmt.Errorf("Can only infer field name from structs")
	}

	return v.Type().Field(index).Name, nil
}

func copyStructValue(to, from reflect.Value) error {
	if to.Kind() != reflect.Struct && from.Kind() != reflect.Struct {
		return fmt.Errorf("Can only copy structs into structs")
	}

	if copyStructSkipField(to, from) {
		return nil
	}

	if to.NumField() != from.NumField() {
		return fmt.Errorf("Structures must have the same number of fields")
	}

	for i := 0; i < to.NumField(); i++ {
		var fromFieldIndex int

		// We want to verify that both fields have the same name
		toFieldName, err := structFieldName(to, i)
		if err != nil {
			return err
		}

		fromFieldName, err := structFieldName(from, i)
		if err != nil {
			return err
		}

		// The two fields at index i do not have the same name.
		// Maybe they're not ordered the same way, let's look through
		// the from struct fields and check if we find one that matches
		// the to struct field we're trying to copy into.
		if fromFieldName != toFieldName {
			fieldFound := false
			j := 0

			for j = 0; j < from.NumField(); j++ {
				fieldName, err := structFieldName(from, j)
				if err != nil {
					continue
				}

				if fieldName == toFieldName {
					fieldFound = true
					break
				}
			}

			if !fieldFound {
				return fmt.Errorf("Wrong field names %s vs %s", toFieldName, fromFieldName)
			} else {
				fromFieldIndex = j
			}
		} else {
			fromFieldIndex = i
		}

		if err := copyValue(to.Field(i), from.Field(fromFieldIndex)); err != nil {
			return err
		}
	}

	return nil
}

func copyStruct(to interface{}, from interface{}) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = r.(error)
		}
	}()

	toVal := reflect.ValueOf(to)
	fromVal := reflect.ValueOf(from)

	if toVal.Kind() != reflect.Ptr || toVal.Elem().Kind() != reflect.Struct ||
		fromVal.Kind() != reflect.Ptr || fromVal.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("Arguments must be pointers to structures")
	}

	toVal = toVal.Elem()
	fromVal = fromVal.Elem()

	return copyStructValue(toVal, fromVal)
}

func OCItoGRPC(ociSpec *specs.Spec) (*Spec, error) {
	s := &Spec{}

	err := copyStruct(s, ociSpec)

	return s, err
}

func GRPCtoOCI(grpcSpec *Spec) (*specs.Spec, error) {
	s := &specs.Spec{}

	err := copyStruct(s, grpcSpec)

	return s, err
}
