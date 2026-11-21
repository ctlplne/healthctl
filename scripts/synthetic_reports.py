"""Create compact fictional reports for end-to-end tests using the standard library."""

import io
import zipfile


def pdf(objects):
    data = bytearray(b"%PDF-1.4\n")
    offsets = [0]
    for index, obj in enumerate(objects, start=1):
        offsets.append(len(data))
        data.extend(f"{index} 0 obj\n".encode() + obj + b"\nendobj\n")
    pos = len(data)
    data.extend(f"xref\n0 {len(offsets)}\n0000000000 65535 f \n".encode())
    for x in offsets[1:]:
        data.extend(f"{x:010d} 00000 n \n".encode())
    data.extend(
        f"trailer\n<< /Size {len(offsets)} /Root 1 0 R >>\nstartxref\n{pos}\n%%EOF\n".encode()
    )
    return bytes(data)


def stream(content, attrs=b""):
    return (
        b"<< /Length "
        + str(len(content)).encode()
        + b" "
        + attrs
        + b" >>\nstream\n"
        + content
        + b"\nendstream"
    )


def digital():
    lines = [
        b"BT /F1 18 Tf 40 680 Td (FICTIONAL REPORT) Tj",
        b"0 -50 Td (Leukocyte esterase: Trace) Tj",
        b"0 -40 Td (Collection date: 2026-10-09) Tj ET",
    ]
    return pdf(
        [
            b"<< /Type /Catalog /Pages 2 0 R >>",
            b"<< /Type /Pages /Count 1 /Kids [3 0 R] >>",
            b"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 540 720] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>",
            b"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
            stream(b"\n".join(lines)),
        ]
    )


def scanned(jpeg, width, height):
    attrs = f"/Type /XObject /Subtype /Image /Width {width} /Height {height} /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /DCTDecode".encode()
    return pdf(
        [
            b"<< /Type /Catalog /Pages 2 0 R >>",
            b"<< /Type /Pages /Count 1 /Kids [3 0 R] >>",
            b"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 540 720] /Resources << /XObject << /Im0 4 0 R >> >> /Contents 5 0 R >>",
            stream(jpeg, attrs),
            stream(b"q 540 0 0 720 0 0 cm /Im0 Do Q"),
        ]
    )


def word():
    out = io.BytesIO()
    with zipfile.ZipFile(out, "w", zipfile.ZIP_DEFLATED) as z:
        z.writestr(
            "[Content_Types].xml",
            '<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>',
        )
        z.writestr(
            "_rels/.rels",
            '<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>',
        )
        xml = '<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:tbl>'
        for cells in (
            ["Test", "Result", "Units", "Reference range"],
            ["Synthetic marker", "2.5", "mg/L", "1.0–3.0"],
        ):
            xml += (
                "<w:tr>"
                + "".join(
                    "<w:tc><w:p><w:r><w:t>" + c + "</w:t></w:r></w:p></w:tc>"
                    for c in cells
                )
                + "</w:tr>"
            )
        z.writestr("word/document.xml", xml + "</w:tbl></w:body></w:document>")
    return out.getvalue()
