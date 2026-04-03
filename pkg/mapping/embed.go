package mapping

import "embed"

//go:embed mappings/*.yaml
var embeddedMappings embed.FS
