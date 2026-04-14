package collect

// Side-effect imports register all device parsers for CollectDevice / GoCollector.
import (
	_ "gorancid/pkg/parse/fortigate"
	_ "gorancid/pkg/parse/generic"
	_ "gorancid/pkg/parse/ios"
	_ "gorancid/pkg/parse/iosxr"
	_ "gorancid/pkg/parse/junos"
	_ "gorancid/pkg/parse/nxos"
)
