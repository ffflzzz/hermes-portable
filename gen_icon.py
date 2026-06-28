#!/usr/bin/env python3
"""Generate a Hermes-style ICO icon."""
from PIL import Image
import struct
import os

# Create a 256x256 icon
img = Image.new('RGBA', (256, 256), (0, 0, 0, 0))
pixels = img.load()

# Background circle
for y in range(256):
    for x in range(256):
        dx = x - 128
        dy = y - 128
        dist = (dx*dx + dy*dy) ** 0.5
        if dist <= 120:
            t = dist / 120
            r = int(20 + t * 10)
            g = int(180 + (1 - t) * 75)
            b = int(80 + t * 40)
            a = 255
            pixels[x, y] = (r, g, b, a)

# Draw "H" in white
h_color = (255, 255, 255, 255)
for y in range(50, 206):
    for x in range(85, 105):
        pixels[x, y] = h_color
for y in range(50, 206):
    for x in range(151, 171):
        pixels[x, y] = h_color
for x in range(85, 171):
    for y in range(118, 138):
        pixels[x, y] = h_color

# Save as ICO
sizes = [(16, 16), (32, 32), (48, 48), (64, 64), (128, 128), (256, 256)]
icons = []

for size in sizes:
    small = img.resize(size, Image.LANCZOS)
    buf = small.tobytes('raw', 'RGBA')
    icons.append((size, buf))

outpath = os.path.join(os.path.dirname(os.path.abspath(__file__)), 'hermes_icon.ico')
with open(outpath, 'wb') as f:
    # ICO header: reserved(2) + type(2) + count(2)
    f.write(struct.pack('<HHH', 0, 1, len(icons)))
    
    offset = 6 + 16 * len(icons)
    
    for i, (size, buf) in enumerate(icons):
        w, h = size
        # Image directory entry (16 bytes each)
        f.write(struct.pack('BBBBHHII',
            w if w < 256 else 0,   # width
            h if h < 256 else 0,   # height
            0,                     # color palette
            0,                     # reserved
            1,                     # color planes
            32,                    # bits per pixel
            len(buf),              # image size
            offset                 # data offset
        ))
        f.write(buf)
        offset += len(buf)

print("Icon saved: hermes_icon.ico")
print(f"File size: {offset} bytes")
