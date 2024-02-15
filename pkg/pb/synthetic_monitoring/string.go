// Code generated by "enumer -type=CheckType,CheckClass -trimprefix=CheckType,CheckClass -transform=lower -output=string.go"; DO NOT EDIT.

package synthetic_monitoring

import (
	"fmt"
	"strings"
)

const _CheckTypeName = "dnshttppingtcptraceroutek6multihttpgrpc"

var _CheckTypeIndex = [...]uint8{0, 3, 7, 11, 14, 24, 26, 35, 39}

const _CheckTypeLowerName = "dnshttppingtcptraceroutek6multihttpgrpc"

func (i CheckType) String() string {
	if i < 0 || i >= CheckType(len(_CheckTypeIndex)-1) {
		return fmt.Sprintf("CheckType(%d)", i)
	}
	return _CheckTypeName[_CheckTypeIndex[i]:_CheckTypeIndex[i+1]]
}

// An "invalid array index" compiler error signifies that the constant values have changed.
// Re-run the stringer command to generate them again.
func _CheckTypeNoOp() {
	var x [1]struct{}
	_ = x[CheckTypeDns-(0)]
	_ = x[CheckTypeHttp-(1)]
	_ = x[CheckTypePing-(2)]
	_ = x[CheckTypeTcp-(3)]
	_ = x[CheckTypeTraceroute-(4)]
	_ = x[CheckTypeScripted-(5)]
	_ = x[CheckTypeMultiHttp-(6)]
	_ = x[CheckTypeGrpc-(7)]
}

var _CheckTypeValues = []CheckType{CheckTypeDns, CheckTypeHttp, CheckTypePing, CheckTypeTcp, CheckTypeTraceroute, CheckTypeScripted, CheckTypeMultiHttp, CheckTypeGrpc}

var _CheckTypeNameToValueMap = map[string]CheckType{
	_CheckTypeName[0:3]:        CheckTypeDns,
	_CheckTypeLowerName[0:3]:   CheckTypeDns,
	_CheckTypeName[3:7]:        CheckTypeHttp,
	_CheckTypeLowerName[3:7]:   CheckTypeHttp,
	_CheckTypeName[7:11]:       CheckTypePing,
	_CheckTypeLowerName[7:11]:  CheckTypePing,
	_CheckTypeName[11:14]:      CheckTypeTcp,
	_CheckTypeLowerName[11:14]: CheckTypeTcp,
	_CheckTypeName[14:24]:      CheckTypeTraceroute,
	_CheckTypeLowerName[14:24]: CheckTypeTraceroute,
	_CheckTypeName[24:26]:      CheckTypeScripted,
	_CheckTypeLowerName[24:26]: CheckTypeScripted,
	_CheckTypeName[26:35]:      CheckTypeMultiHttp,
	_CheckTypeLowerName[26:35]: CheckTypeMultiHttp,
	_CheckTypeName[35:39]:      CheckTypeGrpc,
	_CheckTypeLowerName[35:39]: CheckTypeGrpc,
}

var _CheckTypeNames = []string{
	_CheckTypeName[0:3],
	_CheckTypeName[3:7],
	_CheckTypeName[7:11],
	_CheckTypeName[11:14],
	_CheckTypeName[14:24],
	_CheckTypeName[24:26],
	_CheckTypeName[26:35],
	_CheckTypeName[35:39],
}

// CheckTypeString retrieves an enum value from the enum constants string name.
// Throws an error if the param is not part of the enum.
func CheckTypeString(s string) (CheckType, error) {
	if val, ok := _CheckTypeNameToValueMap[s]; ok {
		return val, nil
	}

	if val, ok := _CheckTypeNameToValueMap[strings.ToLower(s)]; ok {
		return val, nil
	}
	return 0, fmt.Errorf("%s does not belong to CheckType values", s)
}

// CheckTypeValues returns all values of the enum
func CheckTypeValues() []CheckType {
	return _CheckTypeValues
}

// CheckTypeStrings returns a slice of all String values of the enum
func CheckTypeStrings() []string {
	strs := make([]string, len(_CheckTypeNames))
	copy(strs, _CheckTypeNames)
	return strs
}

// IsACheckType returns "true" if the value is listed in the enum definition. "false" otherwise
func (i CheckType) IsACheckType() bool {
	for _, v := range _CheckTypeValues {
		if i == v {
			return true
		}
	}
	return false
}

const _CheckClassName = "protocolscripted"

var _CheckClassIndex = [...]uint8{0, 8, 16}

const _CheckClassLowerName = "protocolscripted"

func (i CheckClass) String() string {
	if i < 0 || i >= CheckClass(len(_CheckClassIndex)-1) {
		return fmt.Sprintf("CheckClass(%d)", i)
	}
	return _CheckClassName[_CheckClassIndex[i]:_CheckClassIndex[i+1]]
}

// An "invalid array index" compiler error signifies that the constant values have changed.
// Re-run the stringer command to generate them again.
func _CheckClassNoOp() {
	var x [1]struct{}
	_ = x[CheckClassProtocol-(0)]
	_ = x[CheckClassScripted-(1)]
}

var _CheckClassValues = []CheckClass{CheckClassProtocol, CheckClassScripted}

var _CheckClassNameToValueMap = map[string]CheckClass{
	_CheckClassName[0:8]:       CheckClassProtocol,
	_CheckClassLowerName[0:8]:  CheckClassProtocol,
	_CheckClassName[8:16]:      CheckClassScripted,
	_CheckClassLowerName[8:16]: CheckClassScripted,
}

var _CheckClassNames = []string{
	_CheckClassName[0:8],
	_CheckClassName[8:16],
}

// CheckClassString retrieves an enum value from the enum constants string name.
// Throws an error if the param is not part of the enum.
func CheckClassString(s string) (CheckClass, error) {
	if val, ok := _CheckClassNameToValueMap[s]; ok {
		return val, nil
	}

	if val, ok := _CheckClassNameToValueMap[strings.ToLower(s)]; ok {
		return val, nil
	}
	return 0, fmt.Errorf("%s does not belong to CheckClass values", s)
}

// CheckClassValues returns all values of the enum
func CheckClassValues() []CheckClass {
	return _CheckClassValues
}

// CheckClassStrings returns a slice of all String values of the enum
func CheckClassStrings() []string {
	strs := make([]string, len(_CheckClassNames))
	copy(strs, _CheckClassNames)
	return strs
}

// IsACheckClass returns "true" if the value is listed in the enum definition. "false" otherwise
func (i CheckClass) IsACheckClass() bool {
	for _, v := range _CheckClassValues {
		if i == v {
			return true
		}
	}
	return false
}
