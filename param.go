package herald

// DeepCopyParam returns a deep copied json param object
func DeepCopyParam(param interface{}) interface{} {
	paramSlice, ok := param.([]interface{})
	if ok {
		var resultSlice []interface{}
		for _, value := range paramSlice {
			resultSlice = append(resultSlice, DeepCopyParam(value))
		}
		return resultSlice
	}

	paramMap, ok := param.(map[string]interface{})
	if ok {
		resultMap := make(map[string]interface{})
		for key, value := range paramMap {
			resultMap[key] = DeepCopyParam(value)
		}
		return resultMap
	}

	return param
}

// DeepCopyMapParam returns a deep copied map param object
func DeepCopyMapParam(param map[string]interface{}) map[string]interface{} {
	paramNew, _ := DeepCopyParam(param).(map[string]interface{})
	return paramNew
}

// UpdateMapParam merges the two maps
func UpdateMapParam(mapOrigin, mapNew map[string]interface{}) {
	for k, v := range mapNew {
		mapOrigin[k] = DeepCopyParam(v)
	}
}
