package filebeat

import (
  "regexp"
)

// Removes prospectors from filebeat configuration that have paths that do not match list of allowed paths.
// Removed prospectors are logged as warnings to supplied logger.
func PruneDisallowedProspectors(config map[string]interface{}, allowedPaths []string) bool {
  if prospectors, ok := getProspectors(config).([]map[string]interface{}); ok {
    acceptedProspectors := make([]interface{}, 0)
    for target, prospector := range prospectors {
      log.Debugf("Checking prospector [%s] configuration for allowed paths.", prospector)
      // Check that all paths of prospector are allowed
      if paths, ok := prospector["paths"].([]interface{}); ok {
        var allowed bool = true
        for _, path := range paths {
          if !isPathAllowed(path.(string), allowedPaths) {
            log.Warnf("Prospector %s is not allowed due to local path restriction for [%s].\n", prospectors[target], path)
            allowed = false
            break
          }
        }
        if allowed {
          acceptedProspectors = append(acceptedProspectors, prospector)
        }
      } else {
        log.Warnf("Could not locate paths for prospector.")
        return false
      }
    }

    setProspectors(config, acceptedProspectors)
    return true
  } else {
    log.Warnf("Could not locate prospectors configuration.")
    return false
  }
}

// Checks if given path is found in given list of allowed paths
func isPathAllowed(path string, allowedPaths []string) bool {
  if allowedPaths == nil || len(allowedPaths) == 0 {
    return true
  }
  for _, allowedPath := range allowedPaths {
    match, _ := regexp.MatchString(allowedPath, path)
    if (match) {
      return true
    }
  }
  return false
}

// Sets prospectors of filebeat configuration
func setProspectors(config interface{}, newProspectors interface{}) {
  filebeat := getElement(config, "filebeat")
  filebeat.(map[string]interface{})["prospectors"] = newProspectors
}

// Gets prospectors from given filebeat configuration
func getProspectors(config  interface{}) interface{} {
  return getElement(config, "filebeat", "prospectors")
}

// Retrieves element from given path in map of maps structure (e.g. YAML model)
func getElement(config interface{}, path ...string) interface{} {
  var object interface{} = config
  for target := 0; target < len(path); target++ {
		if mmap, ok := object.(map[string]interface{}); ok {
      object = mmap[path[target]]
    } else {
      log.Warnf("Unable to retrieve element %s from configuration - parent is not a map[string].", path[target])
      return nil
    }
	}
	return object
}
