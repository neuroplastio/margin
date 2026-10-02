//! margin's mermaid renderer: a WASI command that reads mermaid source on
//! stdin and writes one SVG to stdout. A failure is a message on stderr and a
//! non-zero exit. See ../README.md for how it is built and why it looks the
//! way it does.

mod inter_metrics;

use std::io::{Read, Write};
use std::process::ExitCode;

use merman::svg::{
    CssOverridePolicy, DeterministicTextMeasurer, HostTheme, HostThemeAppearance,
    MeasurementProfileId, Presentation, SvgOutputPolicy, SvgPipelinePreset, SvgRenderOptions,
    TextMeasurementPolicy, TextMeasurementProfile, TextMeasurementProfileIdentity, TextStyle,
    ThemeRole,
};
use merman::{
    Engine, MermaidConfig, OperationControl, RenderOutput, RenderRequest, Renderer,
    SvgEnvironment, SvgRequest,
};

/// The family every label is measured in and painted with. Inter is first
/// because its advances are what `inter_width` measures; IBM Plex Sans is the
/// HOTTY stylesheet's prose font, next in line if Inter is missing.
const FONT_FAMILY: &str = "Inter, IBM Plex Sans, system-ui, sans-serif";

/// margin's cell palette by xterm-256 value, the same one hotdoc.go's
/// stylesheet uses, so a diagram reads as part of the page.
const PALETTE: &[(ThemeRole, &str)] = &[
    (ThemeRole::Canvas, "#1c1c1c"),              // 234, what edge labels mask lines with
    (ThemeRole::EdgeLabelBackground, "#1c1c1c"), // 234
    (ThemeRole::Surface, "#262626"),             // 235, node fill
    (ThemeRole::SurfaceAlt, "#303030"),          // 236, subgraphs, selection
    (ThemeRole::SurfaceMuted, "#262626"),        // 235
    (ThemeRole::Text, "#d0d0d0"),                // 252, text
    (ThemeRole::SubtleText, "#808080"),          // 244, dim
    (ThemeRole::Border, "#808080"),              // 244, node outlines
    (ThemeRole::Line, "#87afff"),                // 111, links: edges and arrowheads
    (ThemeRole::NoteBackground, "#303030"),      // 236
    (ThemeRole::NoteBorder, "#ffaf87"),          // 216, inline code
    (ThemeRole::NoteText, "#d0d0d0"),            // 252
];

/// Series colours, for what mermaid colours by index: pie slices, mindmap
/// and timeline branches, git branches, xychart plots. margin's accents
/// first (links, h1, inline code, h2), then the xterm-256 pastels beside
/// them. merman picks a readable label colour for each.
const SERIES: &[&str] = &[
    "#87afff", // 111
    "#ff87d7", // 212
    "#ffaf87", // 216
    "#d7afff", // 183
    "#87d7af", // 115
    "#d7d787", // 186
    "#87d7d7", // 116
    "#ff8787", // 210
];

/// Journey sections and tasks are filled with the series colours but
/// lettered in the text colour, unreadable on the pastels above; mermaid's
/// fillType0..7 colour only those, so they get the series' dark xterm-256
/// counterparts instead.
const JOURNEY_FILLS: &[&str] = &[
    "#005f87", // 24
    "#87005f", // 89
    "#875f00", // 94
    "#5f0087", // 54
    "#005f5f", // 23
    "#5f5f00", // 58
    "#00875f", // 29
    "#870000", // 88
];

fn main() -> ExitCode {
    let mut src = String::new();
    if let Err(err) = std::io::stdin().read_to_string(&mut src) {
        eprintln!("read mermaid source: {err}");
        return ExitCode::FAILURE;
    }
    match render(&src) {
        Ok(svg) => {
            let mut out = std::io::stdout().lock();
            if let Err(err) = out.write_all(svg.as_bytes()).and_then(|()| out.flush()) {
                eprintln!("write svg: {err}");
                return ExitCode::FAILURE;
            }
            ExitCode::SUCCESS
        }
        Err(msg) => {
            eprintln!("{msg}");
            ExitCode::FAILURE
        }
    }
}

fn render(src: &str) -> Result<String, String> {
    let mut theme = HostTheme::new()
        .with_appearance(HostThemeAppearance::Dark)
        .try_with_font_family(FONT_FAMILY)
        .map_err(|e| e.to_string())?;
    for (role, color) in PALETTE {
        theme = theme.try_with_role(*role, *color).map_err(|e| e.to_string())?;
    }
    let theme = theme
        .try_with_series_palette(SERIES.iter().copied())
        .map_err(|e| e.to_string())?;
    let presentation = Presentation::new().with_theme(theme).resolve();

    // HTML labels first: mermaid's default, and the better-looking result
    // once resvg-safe turns them into <text>. That conversion simulates the
    // CSS cascade at a cost that grows faster than the diagram (a second for
    // an 80-node flowchart) under a fixed work budget merman does not let a
    // host raise, which runs out around 90 nodes. So a large diagram, or one
    // that runs out anyway, is drawn with SVG labels, which need no
    // conversion at any size.
    if statements(src) <= HTML_LABEL_STATEMENTS {
        match render_with(src, &presentation, true) {
            Err(Failure::Merman(merman::RenderError::ResourceLimitExceeded(limit)))
                if limit.id == "svg_fallback_selector_index" => {}
            result => return result.map_err(|f| f.to_string()),
        }
    }
    render_with(src, &presentation, false).map_err(|f| f.to_string())
}

/// The most statements a diagram may have and still get HTML labels.
const HTML_LABEL_STATEMENTS: usize = 60;

/// Lines that say something: not blank, not a %% comment or directive.
fn statements(src: &str) -> usize {
    src.lines()
        .map(str::trim)
        .filter(|l| !l.is_empty() && !l.starts_with("%%"))
        .count()
}

enum Failure {
    Merman(merman::RenderError),
    Setup(String),
    NoDiagram,
}

impl std::fmt::Display for Failure {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Failure::Merman(err) => err.fmt(f),
            Failure::Setup(msg) => f.write_str(msg),
            Failure::NoDiagram => f.write_str("no mermaid diagram found"),
        }
    }
}

fn render_with(
    src: &str,
    presentation: &merman::svg::ResolvedPresentation,
    html_labels: bool,
) -> Result<String, Failure> {
    let setup = |e: &dyn std::fmt::Display| Failure::Setup(e.to_string());

    // Mermaid 12 wraps flowchart and state labels at 120px; 200px (Mermaid
    // 11's width) keeps a short label like "Promote canary" on one line. A
    // document's own %%{init}%% still overrides all of this.
    let mut site = serde_json::json!({
        "flowchart": { "wrappingWidth": 200 },
        "state": { "wrappingWidth": 200 },
    });
    if !html_labels {
        site["htmlLabels"] = false.into();
        site["flowchart"]["htmlLabels"] = false.into();
    }
    for (i, fill) in JOURNEY_FILLS.iter().enumerate() {
        site["themeVariables"][format!("fillType{i}")] = (*fill).into();
    }
    // After the theme, so these win where both set a value.
    let engine = presentation
        .materialize_engine(Engine::new())
        .with_site_config(MermaidConfig::from_value(site));
    let renderer = Renderer::new().with_engine(engine);

    let identity = TextMeasurementProfileIdentity::new(
        MeasurementProfileId::new("margin.inter").map_err(|e| setup(&e))?,
        "inter-4.1-advances",
    )
    .map_err(|e| setup(&e))?;
    let measurer = DeterministicTextMeasurer::default().with_width_callback(inter_width);
    let environment = SvgEnvironment::deterministic().with_text_measurement_policy(
        TextMeasurementPolicy::uniform(TextMeasurementProfile::new(identity, measurer)),
    );

    // resvg-safe: usvg (hotty-blitz) cannot paint <foreignObject>, so every
    // HTML label becomes plain <text>. The root stays transparent; the
    // surface behind it is the terminal's own background.
    let output = SvgOutputPolicy {
        preset: SvgPipelinePreset::ResvgSafe,
        css_override_policy: CssOverridePolicy::StripExistingImportant,
        root_background_color: Some("transparent".into()),
        ..SvgOutputPolicy::default()
    };
    let request = SvgRequest {
        environment,
        pipeline: Some(output.pipeline()),
        presentation: presentation.render_policy(),
        options: SvgRenderOptions {
            // Ids (markers, clip paths, the CSS scope) are prefixed with
            // this, so two diagrams inlined in one DOM do not collide.
            diagram_id: Some(format!("mermaid-{:016x}", fnv1a(src.as_bytes()))),
            ..SvgRenderOptions::default()
        },
        ..SvgRequest::default()
    };

    match renderer.render(RenderRequest::svg(src, OperationControl::new(), request)) {
        Ok(RenderOutput::Svg(Some(svg))) => Ok(size_root(&one_font(svg.svg()))),
        Ok(_) => Err(Failure::NoDiagram),
        Err(err) => Err(Failure::Merman(err)),
    }
}

/// Width of `text` in px as Inter paints it: the sum of each character's
/// advance (kerning ignored, which only ever overestimates a little).
/// Characters Inter lacks count as one em when wide (CJK, emoji) and 0.6em
/// otherwise.
fn inter_width(text: &str, style: &TextStyle) -> f64 {
    use inter_metrics::{ADVANCES, UNITS_PER_EM};
    let bold = is_bold(style);
    let mut units = 0.0;
    for ch in text.chars() {
        let cp = ch as u32;
        let known = u16::try_from(cp)
            .ok()
            .and_then(|cp| ADVANCES.binary_search_by_key(&cp, |e| e.0).ok())
            .map(|i| f64::from(if bold { ADVANCES[i].2 } else { ADVANCES[i].1 }));
        units += known.unwrap_or(if ch.is_control() {
            0.0
        } else if is_wide(cp) {
            UNITS_PER_EM
        } else {
            0.6 * UNITS_PER_EM
        });
    }
    units / UNITS_PER_EM * style.font_size.max(1.0)
}

fn is_bold(style: &TextStyle) -> bool {
    match style.font_weight.as_deref().map(str::trim) {
        Some("bold" | "bolder") => true,
        Some(w) => w.parse::<u32>().is_ok_and(|n| n >= 600),
        None => false,
    }
}

fn is_wide(cp: u32) -> bool {
    matches!(cp,
        0x1100..=0x115F | 0x2E80..=0xA4CF | 0xAC00..=0xD7A3 | 0xF900..=0xFAFF
        | 0xFE30..=0xFE4F | 0xFF00..=0xFF60 | 0xFFE0..=0xFFE6
        | 0x1F300..=0x1FAFF | 0x20000..=0x3FFFD)
}

fn fnv1a(bytes: &[u8]) -> u64 {
    bytes.iter().fold(0xcbf2_9ce4_8422_2325, |h, b| {
        (h ^ u64::from(*b)).wrapping_mul(0x0100_0000_01b3)
    })
}

/// Font lists some kinds hard-code upstream (journey, gitGraph, gantt),
/// where the theme's font does not reach. usvg reads the first declaration
/// and knows no var(), so these would paint in whatever the host's fallback
/// is, though every label was measured as Inter.
const STRAY_FONTS: &[&str] = &[
    "&quot;trebuchet ms&quot;, verdana, arial, sans-serif",
    "'trebuchet ms',verdana,arial,sans-serif",
    "\"trebuchet ms\", verdana, arial, sans-serif",
    "&quot;Open Sans&quot;, sans-serif",
];

/// Paints every label in FONT_FAMILY.
fn one_font(svg: &str) -> String {
    let mut out = svg.to_string();
    for stray in STRAY_FONTS {
        out = out.replace(stray, FONT_FAMILY);
    }
    out.replace("font-family:sans-serif", &format!("font-family:{FONT_FAMILY}"))
        .replace("font-family=\"sans-serif\"", &format!("font-family=\"{FONT_FAMILY}\""))
}

/// Gives the root <svg> its natural size in px. Mermaid emits
/// width="100%" plus a max-width style (or, per diagram kind, explicit
/// sizes), which leaves a host without a container no size to lay out at.
/// This sets width and height to the viewBox's and drops max-width, so every
/// kind comes out the same way.
fn size_root(svg: &str) -> String {
    let Some(start) = svg.find("<svg") else { return svg.to_string() };
    let Some(len) = svg[start..].find('>') else { return svg.to_string() };
    let end = start + len; // index of the tag's '>'
    let tag = &svg[start + 4..end];
    let self_closing = tag.ends_with('/');
    let tag = tag.trim_end_matches('/');

    let attrs = parse_attrs(tag);
    let Some((w, h)) = attrs
        .iter()
        .find(|(k, _)| k == "viewBox")
        .and_then(|(_, v)| view_box_size(v))
    else {
        return svg.to_string();
    };

    let mut out = String::with_capacity(svg.len() + 32);
    out.push_str(&svg[..start]);
    out.push_str("<svg");
    for (k, v) in &attrs {
        match k.as_str() {
            "width" | "height" => continue,
            "style" => {
                let style: Vec<&str> = v
                    .split(';')
                    .map(str::trim)
                    .filter(|d| !d.is_empty() && !d.starts_with("max-width"))
                    .collect();
                if !style.is_empty() {
                    out.push_str(&format!(" style=\"{}\"", style.join(";")));
                }
            }
            _ => out.push_str(&format!(" {k}=\"{v}\"")),
        }
    }
    out.push_str(&format!(" width=\"{}\" height=\"{}\"", px(w), px(h)));
    if self_closing {
        out.push('/');
    }
    out.push_str(&svg[end..]);
    out
}

/// Attributes of a start tag's body, values as written (still escaped).
fn parse_attrs(tag: &str) -> Vec<(String, String)> {
    let mut attrs = Vec::new();
    let mut rest = tag;
    loop {
        rest = rest.trim_start();
        let Some(eq) = rest.find('=') else { break };
        let name = rest[..eq].trim().to_string();
        rest = rest[eq + 1..].trim_start();
        let Some(quote) = rest.chars().next().filter(|c| *c == '"' || *c == '\'') else { break };
        let Some(close) = rest[1..].find(quote) else { break };
        attrs.push((name, rest[1..1 + close].to_string()));
        rest = &rest[close + 2..];
    }
    attrs
}

fn view_box_size(v: &str) -> Option<(f64, f64)> {
    let nums: Vec<f64> = v
        .split(|c: char| c == ',' || c.is_whitespace())
        .filter(|s| !s.is_empty())
        .map(str::parse)
        .collect::<Result<_, _>>()
        .ok()?;
    match nums[..] {
        [_, _, w, h] if w > 0.0 && h > 0.0 && w.is_finite() && h.is_finite() => Some((w, h)),
        _ => None,
    }
}

/// A px length rounded up to a hundredth, without trailing zeros.
fn px(v: f64) -> String {
    let s = format!("{:.2}", (v * 100.0).ceil() / 100.0);
    s.trim_end_matches('0').trim_end_matches('.').to_string()
}
