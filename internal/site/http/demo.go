package http

import "github.com/Yacobolo/leapview/pkg/pagestream"

func visualShowcasePatch() pagestream.SignalPatch {
	return pagestream.SignalPatch{"visuals": visualDocumentation.Showcase}
}
