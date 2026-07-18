// Intermasq - Web panel for dnsmasq
// Copyright (C) 2026 AlexRus1234
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

package main

import "sort"

// configTemplates — предзаполненные шаблоны для POST /api/config/file.
// Цель — дать админу стартовую точку вместо пустого листа при создании
// нового конфига. Шаблоны консервативны:
//   - активны только безопасные boolean-директивы (domain-needed, bogus-priv…)
//   - директивы со значениями (dhcp-range, server, address) закомментированы
//     с примером; админ раскомментирует и подставит свои значения
//   - каждый шаблон начинается с маркера "# === Managed by Intermasq ==="
//
// Добавить новый шаблон = добавить entry в эту map. ID обязан быть lowercase
// с дефисами (нормализуется в handler).
//
// Шаблоны не проходят dnsmasq --test при создании файла (это скелет), но при
// последующем PUT /api/config валидация работает как обычно. Содержимое каждого
// шаблона синтаксически валидно — см. TestConfigTemplatesValidForDnsmasqSyntax.
var configTemplates = map[string]string{
	"empty": "# === Managed by Intermasq ===\n",
	"basic-dhcp": `# === Managed by Intermasq ===
# Базовый DHCP/DNS сервер. Раскомментируй и поправь значения под свою сеть.

domain-needed
bogus-priv
expand-hosts
domain=lan
#dhcp-range=192.168.1.50,192.168.1.150,255.255.255.0,12h
#dhcp-option=option:router,192.168.1.1
#dhcp-option=option:dns-server,192.168.1.1
`,
	"forwarder": `# === Managed by Intermasq ===
# DNS forwarder: резолвинг через внешние upstream'ы без локальных зон.

domain-needed
bogus-priv
no-resolv
strict-order
#server=1.1.1.1
#server=8.8.8.8
#address=/nas.lan/192.168.1.10
`,
	"pxe": `# === Managed by Intermasq ===
# Сетевая загрузка PXE. Дополни базовым DHCP (см. шаблон basic-dhcp).

#dhcp-match=set:efi-x86_64,option:client-arch,7
#dhcp-match=set:legacy-x86,option:client-arch,0
#dhcp-boot=tag:efi-x86_64,syslinux.efi
#dhcp-boot=tag:legacy-x86,pxelinux.0
#pxe-service=tag:efi-x86_64,X86-64_EFI,syslinux.efi
`,
	"aliases": `# === Managed by Intermasq ===
# DNS aliases: address=/domain/IP и cname=alias,target.
# Альтернатива — отдельный файл 10-dns-aliases.conf, который Intermasq
# создаёт автоматически при первом добавлении алиаса через UI.

#address=/nas.lan/192.168.1.10
#cname=wiki,nas.lan
`,
}

// knownConfigTemplateIDs возвращает отсортированный список доступных ID шаблонов.
// Используется для ответа GET /api/config/templates и для подсказки в ошибке
// при неизвестном template в POST /api/config/file.
func knownConfigTemplateIDs() []string {
	out := make([]string, 0, len(configTemplates))
	for k := range configTemplates {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
