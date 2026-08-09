# Sistema pluginov

Plugini sut sidecar-procesy, ktore komunicirajut s panelju črez Unix-sockety.
Svaky plugin jest samostojny HTTP-proces.

Plugini se iščut v `/etc/intermasq/plugins/`; vsaky podkatalog soderži
`manifest.json` i binar:

```json
{"id":"my-plugin","name":"My Plugin","bin":"./plugin-binary"}
```

`id` jest unikalan i služi v URL `/plugins/<id>/*`; `bin` jest relativny put k
izvršajemu fajlu. Pri zapusku `plugins.Load` sozdaje socket-katalog, čita
manifesty, zapuska binar s `INTERMASQ_KEY` i `PLUGIN_SOCKET`, a potom montuje
reverse-proxy. Pri zastavjenju `plugins.Stop` ubiva vse plugin-procesy.

Vsi `/plugins/<id>/*` zaprosy prohodet črez isto autentifikaciju jako API (JWT ili
`X-API-Key`). Plugini se ne prezigrajut avtomatično; posle izměny manifesta ili
binara potrebno restartovati panel.
