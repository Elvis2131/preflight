# AmazonEC2 / eu-west-1 capture (ADR-006 amendment, 2026-10-02)

`amazonec2_euwest1_nat_offer.json` is a REAL extract, not invented data: the six `NAT Gateway`
products and their on-demand terms, plus two real non-NAT (`Compute Instance`) products with their
terms (so the fetch-time filter has something to exclude), copied verbatim from

    https://pricing.us-east-1.amazonaws.com/offers/v1.0/aws/AmazonEC2/20260925174521/eu-west-1/index.json

(441,439,512 bytes; publicationDate 2026-09-25T17:45:21Z; retrieved anonymously on 2026-10-02 — no
credentials). The Reserved terms and the other ~108,000 products of the full file are omitted;
`amazonec2_region_index.json` is a one-region excerpt of the real region index.

Rows (eu-west-1): `EU-NatGateway-Hours` $0.048/Hrs, `EU-NatGateway-Bytes` $0.048/GB,
`EU-RegionalNatGateway-Hours` $0.048/Hrs, `EU-RegionalNatGateway-Bytes` $0.048/GB, and the two
`EU-NatGateway-Prvd-*` (provisioned bandwidth) rows.
