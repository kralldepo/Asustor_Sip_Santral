# Güvenlik Politikası / Security Policy

## Desteklenen sürümler / Supported versions

| Sürüm / Version | Destek / Supported |
|---|---|
| 1.0.x (son sürüm / latest) | Evet / Yes |
| < 1.0 | Hayır / No |

Güvenlik düzeltmeleri yalnız son sürüme yapılır. / Security fixes are made for the latest release only.

## Açık bildirme / Reporting a vulnerability

**Türkçe**

Güvenlik açıklarını herkese açık issue olarak **bildirmeyin**. Bunun yerine GitHub'ın özel bildirim özelliğini kullanın: depo sayfasında **Security → Report a vulnerability**.

Bildiriminize şunları ekleyin:

- Etkilenen SIP Santral sürümü, ADM sürümü ve NAS modeli
- Açığın açıklaması ve olası etkisi
- Yeniden üretme adımları ya da kavram kanıtı
- Biliyorsanız önerilen çözüm

Bu gönüllü bir projedir; bildirimler en kısa sürede incelenir ancak yanıt süresi garanti edilmez. Düzeltme yayınlanana kadar ayrıntıları paylaşmamanızı rica ederiz.

**English**

Please **do not** report security vulnerabilities through public issues. Use GitHub's private reporting instead: **Security → Report a vulnerability** on the repository page.

Please include:

- Affected SIP Santral version, ADM version and NAS model
- A description of the issue and its possible impact
- Steps to reproduce or a proof of concept
- A suggested fix, if you have one

This is a volunteer project; reports are reviewed as soon as possible, but no response time is guaranteed. Please keep details private until a fix is released.

## Güvenli kullanım / Safe operation

- Panel (9490/TCP) ve SIP (5060) portlarını internete açmayın; uzaktan erişim için VPN kullanın. / Do not expose the panel (9490/TCP) or SIP (5060) to the internet; use a VPN for remote access.
- ADM'yi ve SIP Santral'ı güncel tutun. / Keep ADM and SIP Santral up to date.
- Ayar yedeği SIP şifrelerini açık metin içerir; güvenli saklayın. / The settings backup contains SIP passwords in plain text; store it securely.

Ayrıntılar / Details: [README.md](README.md#ağ-ve-güvenlik) · [README.en.md](README.en.md#network-and-security)
