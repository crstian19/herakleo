//! Picks a channel's accent colour from its logo, with tesdash's rule: the
//! most saturated opaque pixel that is neither near-black nor near-white.
//! Mirrors wails/internal/logocolor. The webview cannot do this itself:
//! reading pixels of a cross-origin logo taints the canvas.

use std::{io::Cursor, sync::LazyLock, time::Duration};

use image::{GenericImageView, ImageReader, Limits};

const MAX_LOGO_BYTES: usize = 4 << 20;
/// Larger images are decompression bombs, not logos.
const MAX_LOGO_SIDE: u32 = 4096;
const SAMPLE: u64 = 64;

static CLIENT: LazyLock<reqwest::Client> = LazyLock::new(|| {
    reqwest::Client::builder()
        .timeout(Duration::from_secs(15))
        .user_agent("VLC/3.0.21 LibVLC/3.0.21")
        .build()
        .expect("static reqwest config")
});

/// Downloads a logo and returns its accent as "#rrggbb", or "" when the logo
/// has no usable colour (all transparent, black or white).
pub async fn from_url(url: &str) -> Result<String, String> {
    let u = reqwest::Url::parse(url).map_err(|e| e.to_string())?;
    if !matches!(u.scheme(), "http" | "https") || u.host_str().is_none_or(str::is_empty) {
        return Err("logo URL not http(s)".into());
    }
    let mut resp = CLIENT
        .get(u)
        .send()
        .await
        .map_err(|e| e.without_url().to_string())?;
    if resp.status() != reqwest::StatusCode::OK {
        return Err(format!("logo: {}", resp.status()));
    }
    let mut body = Vec::new();
    while let Some(chunk) = resp
        .chunk()
        .await
        .map_err(|e| e.without_url().to_string())?
    {
        if body.len() + chunk.len() > MAX_LOGO_BYTES {
            return Err("logo too large".into());
        }
        body.extend_from_slice(&chunk);
    }
    // Decoding is CPU work; keep it off the async workers.
    tauri::async_runtime::spawn_blocking(move || from_bytes(&body))
        .await
        .map_err(|e| e.to_string())?
}

/// Decodes PNG, JPEG, GIF or WebP and picks the accent colour.
pub fn from_bytes(data: &[u8]) -> Result<String, String> {
    let mut reader = ImageReader::new(Cursor::new(data))
        .with_guessed_format()
        .map_err(|e| e.to_string())?;
    let mut limits = Limits::default();
    limits.max_image_width = Some(MAX_LOGO_SIDE);
    limits.max_image_height = Some(MAX_LOGO_SIDE);
    reader.limits(limits);
    let img = reader.decode().map_err(|e| e.to_string())?;
    Ok(pick(&img))
}

/// Samples a 64x64 nearest-neighbour grid (same as Go's pick) and keeps the
/// most saturated pixel.
fn pick(img: &image::DynamicImage) -> String {
    let (w, h) = (u64::from(img.width()), u64::from(img.height()));
    if w == 0 || h == 0 {
        return String::new();
    }
    let mut best = None;
    let mut best_score = -1.0;
    for sy in 0..SAMPLE {
        for sx in 0..SAMPLE {
            let x = ((2 * sx + 1) * w / (2 * SAMPLE)) as u32;
            let y = ((2 * sy + 1) * h / (2 * SAMPLE)) as u32;
            let [r, g, b, a] = img.get_pixel(x, y).0;
            if a < 128 {
                continue;
            }
            let (l, s) = light_sat(r, g, b);
            if !(0.15..=0.92).contains(&l) {
                continue;
            }
            if s > best_score {
                best_score = s;
                best = Some((r, g, b));
            }
        }
    }
    best.map_or_else(String::new, |(r, g, b)| format!("#{r:02x}{g:02x}{b:02x}"))
}

/// HSL lightness and saturation, as in tesdash's getSaturation.
fn light_sat(r: u8, g: u8, b: u8) -> (f64, f64) {
    let (rn, gn, bn) = (
        f64::from(r) / 255.0,
        f64::from(g) / 255.0,
        f64::from(b) / 255.0,
    );
    let hi = rn.max(gn).max(bn);
    let lo = rn.min(gn).min(bn);
    let l = (hi + lo) / 2.0;
    if hi == lo {
        return (l, 0.0);
    }
    let d = hi - lo;
    (
        l,
        if l > 0.5 {
            d / (2.0 - hi - lo)
        } else {
            d / (hi + lo)
        },
    )
}

#[cfg(test)]
mod tests {
    use super::*;
    use image::{ImageFormat, Rgba, RgbaImage};

    fn png(img: &RgbaImage) -> Vec<u8> {
        let mut out = Cursor::new(Vec::new());
        img.write_to(&mut out, ImageFormat::Png).unwrap();
        out.into_inner()
    }

    /// Same fixture as logocolor_test.go.
    #[test]
    fn picks_most_saturated() {
        let img = RgbaImage::from_fn(128, 128, |x, y| match y {
            0..32 => Rgba([0, 0, 0, 0]),
            32..48 => Rgba([255, 255, 255, 255]),
            48..64 => Rgba([0, 0, 0, 255]),
            _ if x < 64 => Rgba([220, 30, 40, 255]),
            _ => Rgba([80, 90, 140, 255]),
        });
        assert_eq!(from_bytes(&png(&img)).unwrap(), "#dc1e28");
    }

    #[test]
    fn nothing_usable() {
        assert_eq!(from_bytes(&png(&RgbaImage::new(8, 8))).unwrap(), "");
    }
}
