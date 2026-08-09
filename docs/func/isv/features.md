# Funkcionalne vlastnosti

## DHCP i DNS

- `dhcp-host=` zapisy podržavaju sozdanje, čtenje, izměnu i brisanje s validacijeju MAC, IP i hostname.
- Polja `tags` i `lease_time` sačuvajut se pri masovyh operacijah.
- `GET /api/hosts/next-ip` predlaga svobodny adres iz poznatogo `dhcp-range`.
- DNS tipy **A**, **CNAME**, **PTR** i **TXT** podržavaju standardne operacije i CSV import/export.

## Konfiguracija

Vizualny redaktor rabota s `dhcp-range`, `dhcp-option`, `server=`, PXE i mrežnu
zagruzku. Raw-redaktor dostupny samo roli `admin`; pred zapisom se izvršaje
`dnsmasq --test`. Podržany sut mnogofajlovost i presety `basic-dhcp`, `forwarder`,
`pxe`, `aliases` i `empty`.

## Bezopastnost i masove operacije

Pred zapisom se tvori `.bak`; istorija hrani do 10 verzij na fajl. Dostupny sut
diff, vosstanovjenje, ZIP backup, bulk-move, bulk-edit, lease-to-static i CSV.
`IsSafePath` zapreščaje operacije izvan `-conf-dir`.

## Ustroistva i UI

OUI-lookup opredêlja proizvoditelja po MAC. Frontend jest vgrađen v binar preko
`go:embed`; SSE prenosi ARP i status dnsmasq bez periodičnogo oprašivanija.
Podržany sut JWT, `X-API-Key`, RBAC, Swagger, rusky i anglijski jezyk, a takže
temna i svetla tema.
