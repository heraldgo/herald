package herald

// deepCopyParam returns a deep copied json param object
func deepCopyParam(param interface{}) interface{} {
	paramSlice, ok := param.([]interface{})
	if ok {
		resultSlice := make([]interface{}, 0, len(paramSlice))
		for _, value := range paramSlice {
			resultSlice = append(resultSlice, deepCopyParam(value))
		}
		return resultSlice
	}

	paramMap, ok := param.(map[string]interface{})
	if ok {
		resultMap := make(map[string]interface{})
		for key, value := range paramMap {
			resultMap[key] = deepCopyParam(value)
		}
		return resultMap
	}

	return param
}

// deepCopyMapParam returns a deep copied map param object
func deepCopyMapParam(param map[string]interface{}) map[string]interface{} {
	paramNew, _ := deepCopyParam(param).(map[string]interface{})
	return paramNew
}

// mergeMapParam merges the two maps
func mergeMapParam(mapOrigin, mapNew map[string]interface{}) {
	for k, v := range mapNew {
		mapOrigin[k] = deepCopyParam(v)
	}
}
