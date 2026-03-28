BEGIN;

INSERT INTO regulatory_sources (id, name, source_type, country_code, region, url, rss_feed_url, relevance_frameworks, scan_frequency, is_active) VALUES
-- UK
(gen_random_uuid(), 'ICO (Information Commissioner''s Office)', 'supervisory_authority', 'GB', 'UK', 'https://ico.org.uk', 'https://ico.org.uk/about-the-ico/news-and-events/news-and-blogs/rss/', '{UK_GDPR,CYBER_ESSENTIALS}', 'daily', true),
(gen_random_uuid(), 'NCSC (National Cyber Security Centre)', 'government', 'GB', 'UK', 'https://www.ncsc.gov.uk', 'https://www.ncsc.gov.uk/api/1/services/v1/all-rss-feed.xml', '{NCSC_CAF,CYBER_ESSENTIALS,ISO27001}', 'daily', true),
(gen_random_uuid(), 'FCA (Financial Conduct Authority)', 'supervisory_authority', 'GB', 'UK', 'https://www.fca.org.uk', NULL, '{PCI_DSS_4,UK_GDPR}', 'daily', true),
(gen_random_uuid(), 'PRA (Prudential Regulation Authority)', 'supervisory_authority', 'GB', 'UK', 'https://www.bankofengland.co.uk/prudential-regulation', NULL, '{NIST_800_53,ISO27001}', 'weekly', true),
(gen_random_uuid(), 'Bank of England', 'government', 'GB', 'UK', 'https://www.bankofengland.co.uk', NULL, '{NIST_800_53}', 'weekly', true),

-- EU Institutions
(gen_random_uuid(), 'ENISA (EU Agency for Cybersecurity)', 'government', NULL, 'EU', 'https://www.enisa.europa.eu', 'https://www.enisa.europa.eu/rss.xml', '{NIST_CSF_2,ISO27001,NCSC_CAF}', 'daily', true),
(gen_random_uuid(), 'EDPB (European Data Protection Board)', 'supervisory_authority', NULL, 'EU', 'https://edpb.europa.eu', NULL, '{UK_GDPR}', 'daily', true),
(gen_random_uuid(), 'European Commission - Digital Policy', 'government', NULL, 'EU', 'https://digital-strategy.ec.europa.eu', NULL, '{UK_GDPR,NIST_CSF_2}', 'weekly', true),

-- Germany
(gen_random_uuid(), 'BSI (Bundesamt für Sicherheit in der Informationstechnik)', 'government', 'DE', 'EU', 'https://www.bsi.bund.de', 'https://www.bsi.bund.de/SiteGlobals/Functions/RSSFeed/RSSNewsfeed/RSSNewsfeed.xml', '{ISO27001,NIST_CSF_2,NCSC_CAF}', 'daily', true),
(gen_random_uuid(), 'BfDI (Bundesbeauftragter für den Datenschutz)', 'supervisory_authority', 'DE', 'EU', 'https://www.bfdi.bund.de', NULL, '{UK_GDPR}', 'weekly', true),

-- France
(gen_random_uuid(), 'ANSSI (Agence nationale de la sécurité des systèmes d''information)', 'government', 'FR', 'EU', 'https://www.ssi.gouv.fr', 'https://www.ssi.gouv.fr/feed/actualite/', '{ISO27001,NIST_CSF_2,NCSC_CAF}', 'daily', true),
(gen_random_uuid(), 'CNIL (Commission nationale de l''informatique et des libertés)', 'supervisory_authority', 'FR', 'EU', 'https://www.cnil.fr', 'https://www.cnil.fr/fr/rss.xml', '{UK_GDPR}', 'daily', true),

-- Netherlands
(gen_random_uuid(), 'NCSC-NL (Nationaal Cyber Security Centrum)', 'government', 'NL', 'EU', 'https://www.ncsc.nl', NULL, '{ISO27001,NCSC_CAF}', 'daily', true),
(gen_random_uuid(), 'AP (Autoriteit Persoonsgegevens)', 'supervisory_authority', 'NL', 'EU', 'https://autoriteitpersoonsgegevens.nl', NULL, '{UK_GDPR}', 'weekly', true),

-- Spain
(gen_random_uuid(), 'CCN-CERT (Centro Criptológico Nacional)', 'government', 'ES', 'EU', 'https://www.ccn-cert.cni.es', NULL, '{ISO27001,NIST_CSF_2}', 'weekly', true),
(gen_random_uuid(), 'AEPD (Agencia Española de Protección de Datos)', 'supervisory_authority', 'ES', 'EU', 'https://www.aepd.es', NULL, '{UK_GDPR}', 'weekly', true),

-- Italy
(gen_random_uuid(), 'ACN (Agenzia per la Cybersicurezza Nazionale)', 'government', 'IT', 'EU', 'https://www.acn.gov.it', NULL, '{ISO27001,NIST_CSF_2}', 'weekly', true),

-- International Standards Bodies
(gen_random_uuid(), 'ISO (International Organization for Standardization)', 'standards_body', NULL, 'Global', 'https://www.iso.org', NULL, '{ISO27001}', 'weekly', true),
(gen_random_uuid(), 'NIST (National Institute of Standards and Technology)', 'standards_body', 'US', 'Global', 'https://www.nist.gov', 'https://www.nist.gov/news-events/news/rss.xml', '{NIST_800_53,NIST_CSF_2}', 'daily', true),
(gen_random_uuid(), 'PCI SSC (Payment Card Industry Security Standards Council)', 'standards_body', NULL, 'Global', 'https://www.pcisecuritystandards.org', NULL, '{PCI_DSS_4}', 'weekly', true),
(gen_random_uuid(), 'ISACA', 'standards_body', NULL, 'Global', 'https://www.isaca.org', NULL, '{COBIT_2019}', 'weekly', true),
(gen_random_uuid(), 'AXELOS / PeopleCert', 'standards_body', NULL, 'Global', 'https://www.axelos.com', NULL, '{ITIL_4}', 'monthly', true),

-- Additional EU DPAs
(gen_random_uuid(), 'DPC Ireland (Data Protection Commission)', 'supervisory_authority', 'IE', 'EU', 'https://www.dataprotection.ie', NULL, '{UK_GDPR}', 'weekly', true),
(gen_random_uuid(), 'UODO Poland (Urząd Ochrony Danych Osobowych)', 'supervisory_authority', 'PL', 'EU', 'https://uodo.gov.pl', NULL, '{UK_GDPR}', 'weekly', true);

COMMIT;
