# AWS Architecture Icons — licence position and provenance (PC-109)

**This directory holds AWS's own icons, which are not covered by this repository's licence.**

| | |
|---|---|
| Source | AWS Architecture Icons, <https://aws.amazon.com/architecture/icons/> |
| Package version | `Icon-package_07312026` (the quarterly release dated 2026-07-31; its folders are suffixed `_07312026`) |
| Obtained | Downloaded by the project owner from the source above and placed in the repository's `ui/` directory (the zip is deliberately **not** committed: see `.gitignore`) |
| Committed here | The service, resource, category and boundary icons used by the expanded AWS library (listed in `SHA256SUMS`), copied **byte for byte** |
| Archive paths | `SOURCE-PATHS.json` records each asset's exact location in the source package |

## What AWS says, and what could and could not be verified

Verified directly on the source page (2026-10-01):

> "We allow customers and partners to use these toolkits and assets to create architecture diagrams."
>
> "You can also put icons in materials like whitepapers, presentations, data sheets, and posters."

Verified on AWS's general Trademark Guidelines (<https://aws.amazon.com/trademark-guidelines/>), which govern AWS marks and logos:

> "You will not alter the logo images in any manner, including but not limited to changing the proportion, color, or font of the AWS Marks"
>
> "You will not display any AWS Mark in a way that implies sponsorship or endorsement by AWS other than by using the Marks as specifically authorized"

**Not verified:** the icon package ships with no terms file, and the detailed icon-specific terms were not retrievable from the source page by an automated fetch (the page links only to AWS's general legal, privacy and site-terms pages). Nothing in this file should be read as AWS's full terms. **Before this repository is made public, or the icons are used outside architecture diagrams, read the terms on the source page and confirm they cover this use**; the project owner accepted AWS's terms on download, and that acceptance is theirs, not something this file can give.

## How this project stays inside what was verified

- **Unaltered.** Icons are copied byte for byte and drawn at a fixed square size, so proportion is never changed; none is recoloured, cropped, redrawn or combined with other marks. `SHA256SUMS` records each file's digest and a test fails if any file differs from it.
- **No implied endorsement.** Icons appear only on canvas nodes, the service palette and the Region/VPC/subnet groupings, to say *which AWS service a node represents*. Preflight is not an AWS product and nothing in the UI says or implies AWS sponsors, endorses or verified a design.
- **Presentation only.** The mapping from a service to its icon (`canvas/src/awsIcons.ts`) is a lookup keyed by the capability registry's service ID. Nothing about assessment, simulation or any verdict depends on it.
- **Correct identity.** Services use their official service icons; resource variants use their own resource icon or their parent service's icon. Entries without a dedicated icon in this release use the corresponding AWS category icon, with a tooltip identifying it as a category icon. Unknown entries retain a generic mark. Icons never imply that a service has a backend model.
- **The report diagram stays label-based.** The server-rendered report SVG (`render/`, PC-81) does not embed icons: doing so would make the Graphviz output depend on external image files and their paths and would bloat it, while the golden SVG/HTML fixtures rely on that output being byte-stable. Recorded in `render/testdata/README.md`; the golden fixtures are byte-identical with or without this directory.

## Removing the icons

Delete this directory and `canvas/src/awsIcons.ts`'s mapping; every node falls back to its labelled generic shape. Nothing else depends on them.
