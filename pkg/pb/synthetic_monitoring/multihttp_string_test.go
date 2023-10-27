package synthetic_monitoring

import (
	"cmp"
	fmt "fmt"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/exp/constraints"
)

type enum interface {
	constraints.Integer
	fmt.Stringer
}

// The following tests make sure everything is kept in sync, in particular that
// modifying the protobuf definition doesn't invalidate the string generated code.

// TestEnumToString makes sure that the strings generated from the proto file
// match the strings generated using enumer.
func TestEnumToString(t *testing.T) {
	testEnumToString[MultiHttpEntryAssertionType](t, MultiHttpEntryAssertionType_name)
	testEnumToString[MultiHttpEntryAssertionSubjectVariant](t, MultiHttpEntryAssertionSubjectVariant_name)
	testEnumToString[MultiHttpEntryAssertionConditionVariant](t, MultiHttpEntryAssertionConditionVariant_name)
	testEnumToString[MultiHttpEntryVariableType](t, MultiHttpEntryVariableType_name)
}

func TestStringToEnum(t *testing.T) {
	testStringToEnum(t, MultiHttpEntryAssertionTypeString, MultiHttpEntryAssertionType_name)
	testStringToEnum(t, MultiHttpEntryAssertionSubjectVariantString, MultiHttpEntryAssertionSubjectVariant_name)
	testStringToEnum(t, MultiHttpEntryAssertionConditionVariantString, MultiHttpEntryAssertionConditionVariant_name)
	testStringToEnum(t, MultiHttpEntryVariableTypeString, MultiHttpEntryVariableType_name)
}

func TestAllValuesIncluded(t *testing.T) {
	testAllValuesIncluded(t, MultiHttpEntryAssertionTypeValues(), MultiHttpEntryAssertionType_name)
	testAllValuesIncluded(t, MultiHttpEntryAssertionSubjectVariantValues(), MultiHttpEntryAssertionSubjectVariant_name)
	testAllValuesIncluded(t, MultiHttpEntryAssertionConditionVariantValues(), MultiHttpEntryAssertionConditionVariant_name)
	testAllValuesIncluded(t, MultiHttpEntryVariableTypeValues(), MultiHttpEntryVariableType_name)
}

func TestMultiHttpEntryAssertionType_IsAMultiHttpEntryAssertionType(t *testing.T) {
	testAllValuesAreValid(t, func(v MultiHttpEntryAssertionType) bool {
		return v.IsAMultiHttpEntryAssertionType()
	}, MultiHttpEntryAssertionType_name)

	testAllValuesAreValid(t, func(v MultiHttpEntryAssertionSubjectVariant) bool {
		return v.IsAMultiHttpEntryAssertionSubjectVariant()
	}, MultiHttpEntryAssertionSubjectVariant_name)

	testAllValuesAreValid(t, func(v MultiHttpEntryAssertionConditionVariant) bool {
		return v.IsAMultiHttpEntryAssertionConditionVariant()
	}, MultiHttpEntryAssertionConditionVariant_name)

	testAllValuesAreValid(t, func(v MultiHttpEntryVariableType) bool {
		return v.IsAMultiHttpEntryVariableType()
	}, MultiHttpEntryVariableType_name)
}

func testEnumToString[E enum](t *testing.T, m map[int32]string) {
	t.Helper()
	for v, str := range m {
		v := E(v)
		require.Equal(t, str, v.String())
	}
}

func testStringToEnum[E enum](t *testing.T, f func(string) (E, error), m map[int32]string) {
	t.Helper()
	for v, str := range m {
		expected := E(v)
		actual, err := f(str)
		require.NoError(t, err)
		require.Equal(t, expected, actual)
	}
}

func getValues[E cmp.Ordered](m map[int32]string) []E {
	values := make([]E, 0, len(m))
	for v := range m {
		values = append(values, E(v))
	}
	slices.Sort(values)
	return values
}

func testAllValuesIncluded[E enum](t *testing.T, values []E, m map[int32]string) {
	t.Helper()
	require.Equal(t, getValues[E](m), values)
}

func testAllValuesAreValid[E enum](t *testing.T, f func(E) bool, m map[int32]string) {
	t.Helper()
	values := getValues[E](m)

	for _, v := range values {
		require.True(t, f(v))
	}

	require.False(t, f(values[0]-1))
	require.False(t, f(values[len(values)-1]+1))
}
