// Draws the SIP Santral icon: blue rounded square, white phone handset and a
// red "recording" badge.  Usage: swift tools/icon/icon.swift <out.png> <size>
import Foundation
import CoreGraphics
import ImageIO
import UniformTypeIdentifiers

let out = CommandLine.arguments[1]
let size = Int(CommandLine.arguments[2])!
let cs = CGColorSpaceCreateDeviceRGB()
let ctx = CGContext(data: nil, width: size, height: size, bitsPerComponent: 8, bytesPerRow: 0, space: cs,
                    bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue)!
let s = CGFloat(size) / 256.0
ctx.translateBy(x: 0, y: CGFloat(size)); ctx.scaleBy(x: s, y: -s)  // top-left origin, 256 units
ctx.setShouldAntialias(true)
func rgb(_ r: CGFloat, _ g: CGFloat, _ b: CGFloat, _ a: CGFloat = 1) -> CGColor { CGColor(colorSpace: cs, components: [r/255, g/255, b/255, a])! }

// Background
let bg = CGPath(roundedRect: CGRect(x: 8, y: 8, width: 240, height: 240), cornerWidth: 54, cornerHeight: 54, transform: nil)
ctx.saveGState(); ctx.addPath(bg); ctx.clip()
let grad = CGGradient(colorsSpace: cs, colors: [rgb(28, 40, 70), rgb(16, 24, 42)] as CFArray, locations: [0, 1])!
ctx.drawLinearGradient(grad, start: CGPoint(x: 20, y: 8), end: CGPoint(x: 236, y: 248), options: [])
ctx.restoreGState()

// Handset: a thick curved bar with rounded ear/mouth pieces (rotated 45°)
ctx.saveGState()
ctx.translateBy(x: 120, y: 136); ctx.rotate(by: -.pi / 4)
ctx.setFillColor(rgb(255, 255, 255))
let body = CGMutablePath()
body.move(to: CGPoint(x: -78, y: -10))
body.addCurve(to: CGPoint(x: 78, y: -10), control1: CGPoint(x: -40, y: 22), control2: CGPoint(x: 40, y: 22))
body.addLine(to: CGPoint(x: 78, y: 18))
body.addCurve(to: CGPoint(x: -78, y: 18), control1: CGPoint(x: 40, y: 52), control2: CGPoint(x: -40, y: 52))
body.closeSubpath()
ctx.addPath(body); ctx.fillPath()
for x in [CGFloat(-74), 74] {
    let p = CGPath(roundedRect: CGRect(x: x - 26, y: -34, width: 52, height: 50), cornerWidth: 18, cornerHeight: 18, transform: nil)
    ctx.addPath(p); ctx.fillPath()
}
ctx.restoreGState()

// Incoming arrow (top-left) in light blue
ctx.setStrokeColor(rgb(127, 166, 255)); ctx.setLineCap(.round); ctx.setLineJoin(.round); ctx.setLineWidth(14)
ctx.move(to: CGPoint(x: 112, y: 52)); ctx.addLine(to: CGPoint(x: 64, y: 100)); ctx.strokePath()
ctx.move(to: CGPoint(x: 64, y: 62)); ctx.addLine(to: CGPoint(x: 64, y: 100)); ctx.addLine(to: CGPoint(x: 102, y: 100)); ctx.strokePath()

// Recording badge
let c = CGPoint(x: 190, y: 190), r: CGFloat = 38
ctx.setFillColor(rgb(255, 255, 255)); ctx.fillEllipse(in: CGRect(x: c.x - r, y: c.y - r, width: 2 * r, height: 2 * r))
ctx.setFillColor(rgb(211, 58, 58)); ctx.fillEllipse(in: CGRect(x: c.x - 22, y: c.y - 22, width: 44, height: 44))

let img = ctx.makeImage()!
let dest = CGImageDestinationCreateWithURL(URL(fileURLWithPath: out) as CFURL, UTType.png.identifier as CFString, 1, nil)!
CGImageDestinationAddImage(dest, img, nil)
guard CGImageDestinationFinalize(dest) else { fatalError("png yazılamadı") }
print("ok \(out) \(size)x\(size)")
