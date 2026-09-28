"""Draw the DAWGit app icon: two versions branching from one and merging back.

    python build/make_icon.py        (from desktop/)

Writes build/appicon.png (1024x1024). Then generate the .ico with
    wails3 generate icons -input build/appicon.png -windowsfilename build/windows/icon.ico
"""

from pathlib import Path

from PIL import Image, ImageDraw

SIZE = 1024
SS = 4  # supersampling for smooth edges
S = SIZE * SS


def lerp(a, b, t):
    return tuple(round(x + (y - x) * t) for x, y in zip(a, b))


def main():
    # Diagonal teal gradient.
    top, bottom = (72, 214, 186), (22, 110, 98)
    grad = Image.new("RGB", (S, S))
    px = grad.load()
    for y in range(0, S, SS):
        for x in range(0, S, SS):
            c = lerp(top, bottom, (x + y) / (2 * S))
            for dy in range(SS):
                for dx in range(SS):
                    px[x + dx, y + dy] = c

    mask = Image.new("L", (S, S), 0)
    ImageDraw.Draw(mask).rounded_rectangle([0, 0, S - 1, S - 1], radius=int(S * 0.22), fill=255)
    icon = Image.new("RGBA", (S, S), (0, 0, 0, 0))
    icon.paste(grad, (0, 0), mask)

    d = ImageDraw.Draw(icon)
    white = (255, 255, 255, 255)
    w = int(S * 0.085)  # stroke width
    cx_main, cx_side = int(S * 0.38), int(S * 0.66)  # optically centred
    y_top, y_bot = int(S * 0.24), int(S * 0.76)
    y_split, y_join = int(S * 0.64), int(S * 0.36)

    # Main line.
    d.line([(cx_main, y_top), (cx_main, y_bot)], fill=white, width=w)
    # Side line: leaves the main line, runs up, rejoins at the top.
    steps = 48

    def curve(p0, p1, p2, p3):
        pts = []
        for i in range(steps + 1):
            t = i / steps
            mt = 1 - t
            pts.append((
                mt**3 * p0[0] + 3 * mt**2 * t * p1[0] + 3 * mt * t**2 * p2[0] + t**3 * p3[0],
                mt**3 * p0[1] + 3 * mt**2 * t * p1[1] + 3 * mt * t**2 * p2[1] + t**3 * p3[1],
            ))
        return pts

    low = curve((cx_main, y_split + int(S * .08)), (cx_main, y_split - int(S * .02)),
                (cx_side, y_split + int(S * .06)), (cx_side, y_split - int(S * .06)))
    high = curve((cx_side, y_join + int(S * .06)), (cx_side, y_join - int(S * .06)),
                 (cx_main, y_join + int(S * .02)), (cx_main, y_join - int(S * .08)))
    straight = [(cx_side, y_split - int(S * .06) + (y_join - y_split + int(S * .12)) * i / steps) for i in range(steps + 1)]
    path = low + straight + high
    # Stroke by stamping round dots densely: smooth joins at any width.
    r = w // 2
    for (x0, y0), (x1, y1) in zip(path, path[1:]):
        n = max(1, int(((x1 - x0) ** 2 + (y1 - y0) ** 2) ** 0.5 / (r / 4)))
        for i in range(n + 1):
            x, y = x0 + (x1 - x0) * i / n, y0 + (y1 - y0) * i / n
            d.ellipse([x - r, y - r, x + r, y + r], fill=white)

    # Version dots: base (bottom), side version, merge (top).
    def dot(x, y, r):
        d.ellipse([x - r, y - r, x + r, y + r], fill=white)

    big = int(S * 0.085)
    dot(cx_main, y_bot, big)
    dot(cx_main, y_top, big)
    dot(cx_side, (y_split + y_join) // 2, int(S * 0.07))

    out = icon.resize((SIZE, SIZE), Image.LANCZOS)
    here = Path(__file__).resolve().parent
    out.save(here / "appicon.png")
    out.resize((256, 256), Image.LANCZOS).save(here.parent / "frontend" / "public" / "icon.png")
    print("wrote build/appicon.png and frontend/public/icon.png")


if __name__ == "__main__":
    main()
