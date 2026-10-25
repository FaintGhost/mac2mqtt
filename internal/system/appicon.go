package system

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
)

// appIconScript draws an app's icon, as Finder shows it, into a PNG of
// 128 pixels, printed in base64: what plays, as the window shows it.
const appIconScript = `ObjC.import('AppKit');
function run(argv) {
  const ws = $.NSWorkspace.sharedWorkspace;
  const url = ws.URLForApplicationWithBundleIdentifier(argv[0]);
  if (url.isNil()) return '';
  const icon = ws.iconForFile(url.path);
  const rep = $.NSBitmapImageRep.alloc.initWithBitmapDataPlanesPixelsWidePixelsHighBitsPerSampleSamplesPerPixelHasAlphaIsPlanarColorSpaceNameBytesPerRowBitsPerPixel(
    null, 128, 128, 8, 4, true, false, 'NSDeviceRGBColorSpace', 0, 0);
  $.NSGraphicsContext.saveGraphicsState;
  $.NSGraphicsContext.setCurrentContext($.NSGraphicsContext.graphicsContextWithBitmapImageRep(rep));
  icon.drawInRectFromRectOperationFraction($.NSMakeRect(0, 0, 128, 128), $.NSMakeRect(0, 0, 0, 0), 1, 1);
  $.NSGraphicsContext.restoreGraphicsState;
  return ObjC.unwrap(rep.representationUsingTypeProperties(4, $()).base64EncodedStringWithOptions(0));
}`

// AppIcon returns the PNG of the icon of the app with the bundle ID.
func (c *Collector) AppIcon(ctx context.Context, bundleID string) ([]byte, error) {
	if c.goos != "darwin" || bundleID == "" {
		return nil, ErrUnsupported
	}
	output, err := runCommand(ctx, c.runner, "/usr/bin/osascript", "-l", "JavaScript", "-e", appIconScript, bundleID)
	if err != nil {
		return nil, err
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(output)))
	if err != nil || len(data) == 0 {
		return nil, errors.New("system: no icon for " + bundleID)
	}
	return data, nil
}
