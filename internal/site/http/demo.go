package http

import "github.com/Yacobolo/libredash/pkg/pagestream"

func visualShowcasePatch() pagestream.SignalPatch {
	return pagestream.SignalPatch{"visuals": visualDocumentation.Showcase}
}
