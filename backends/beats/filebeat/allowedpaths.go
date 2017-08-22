package filebeat

import (
  "strings"
)

// Removes prospectors from filebeat configuration that have paths that do not match list of allowed paths.
// Removed prospectors are logged as warnings to supplied logger.
func PruneDisallowedProspectors(config map[string]interface{}, allowedPaths []string) {
  log.Debugf("Allowed paths is %s.", allowedPaths)
  newProspectors := make([]interface{}, 0)
  if prospectors, ok := getProspectors(config).([]map[string]interface{}); ok {
    for target, prospector := range prospectors {
      log.Debugf("Checking prospector [%s] configuration for allowed paths.", prospector)
      // Check that all paths of prospector are allowed
      var allowed bool = true
      for _, path := range prospector["paths"].([]interface{}) {
        if !isPathAllowed(path.(string), allowedPaths) {
          log.Warnf("Prospector %s is not allowed due to local path restriction for [%s].\n", prospectors[target], path)
          allowed = false
          break
        }
      }
      if allowed {
        newProspectors = append(newProspectors, prospector)
      }
    }

    setProspectors(config, newProspectors)
  } else {
    log.Warnf("Could not locate prospectors configuration.")
  }
}

// Checks if given path is found in given list of allowed paths
func isPathAllowed(path string, allowedPaths []string) bool {
  if allowedPaths == nil || len(allowedPaths) == 0 {
    return true
  }
  for _, allowedPath := range allowedPaths {
    if strings.HasPrefix(path, allowedPath) {
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
    }
	}
	return object
}
