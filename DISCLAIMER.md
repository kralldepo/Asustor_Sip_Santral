# Sorumluluk Reddi / Disclaimer

**Türkçe** · [English](#english)

## Türkçe

Bu belge bilgilendirme amaçlıdır ve hukuki tavsiye değildir. Emin olmadığınız konularda bir hukukçuya danışın.

### 1. Garanti yoktur

SIP Santral, [MIT lisansı](LICENSE) kapsamında **"olduğu gibi"** sunulur. Yazılımın kesintisiz, hatasız ya da belirli bir amaca uygun çalışacağı garanti edilmez. Yazılımın kullanılmasından ya da kullanılamamasından doğan doğrudan veya dolaylı zararlardan (kaçırılan aramalar, kaybolan ya da okunamayan kayıtlar, hizmet kesintisi, iş kaybı, telefon faturaları, veri ihlali dahil) geliştiriciler ve katkıda bulunanlar sorumlu tutulamaz.

### 2. Arama kaydı ve kişisel veriler

- SIP Santral cevaplanan aramaları baştan sona kaydeder. Kayıtları hukuka uygun yapmanın ve saklamanın sorumluluğu, sistemi kuran ve işleten kişi ya da kuruma (veri sorumlusuna) aittir.
- **Türkiye:** 6698 sayılı Kişisel Verilerin Korunması Kanunu (KVKK) arayanın aydınlatılmasını, kaydın hukuki bir sebebe dayanmasını ve verinin gerektiği kadar saklanmasını gerektirir.
- **Avrupa Birliği:** GDPR, kayıt için hukuki dayanak (md. 6) ve arayanın bilgilendirilmesini (md. 13) şart koşar.
- **Almanya:** Ayrıca StGB §201 uyarınca rıza olmadan konuşmanın kaydedilmesi suç oluşturabilir.
- **Diğer ülkeler:** Bazı ülkelerde konuşmadaki tarafların bir kısmının ya da tamamının onayı gerekir.
- Çalışanlarınızın görüşmeleri de kaydedilir. İş hukuku ve çalışan bilgilendirme kurallarına (ör. Almanya'da işyeri konseyi onayı) uyun.
- Öneriler: karşılama anonsunda kayıt yapıldığını söyleyin, saklama süresini gerektiği kadar tutun, kayıt klasörüne yalnız yetkili kişilerin erişmesini sağlayın, gerekmeyen hatlarda kaydı kapatın.

### 3. Acil aramalar

SIP Santral giden arama yapmaz; 112 ya da başka bir acil numarayı aramak için **kullanılamaz**. NAS, elektrik, internet ya da sağlayıcı kesintisinde gelen aramalar da karşılanamaz. Kritik hatlar için yedek bir çözümünüz olsun.

### 4. Güvenlik

- NAS'ın, ağın, portların ve şifrelerin güvenliği işletenin sorumluluğundadır. ADM'yi güncel tutun, güçlü parolalar kullanın, panel (9490) ve SIP (5060) portlarını internete açmayın, uzaktan erişim için VPN kullanın.
- SIP hesap bilgileri NAS'ta saklanır. Ayar yedeği SIP ve dahili şifrelerini açık metin olarak içerir.
- Yanlış yapılandırma ya da yetkisiz erişim sonucu oluşan ücretlerden (ör. SIP hesabının kötüye kullanılması) sorumluluk kabul edilmez.

### 5. Üçüncü taraf yazılım ve hizmetler

- APK içindeki üçüncü taraf bileşenler kendi lisanslarına tabidir; bkz. [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
- Piper ses modelleri kullanıcı isteğiyle Hugging Face'ten indirilir. Her modelin kendi lisansı vardır; bazıları ticari kullanımı kısıtlar.
- SIP sağlayıcınızın, ASUSTOR'un (ADM, App Central), STUN sunucularının (Google, Cloudflare) ve Hugging Face'in kendi kullanım koşulları geçerlidir.

### 6. Ticari markalar ve bağımsızlık

Bu proje bağımsızdır. ASUSTOR Inc., Sangoma Technologies ya da adı geçen diğer şirketlerle bağlantılı değildir; onlar tarafından desteklenmez ya da onaylanmaz. ASUSTOR, ADM ve App Central, ASUSTOR Inc.'in; Asterisk, Sangoma Technologies'in ticari markalarıdır. MicroSIP, Zoiper, Linphone ve diğer adlar sahiplerine aittir ve yalnız uyumluluğu anlatmak için kullanılır.

---

## English

This document is for information only and is not legal advice. Consult a lawyer if you are unsure.

### 1. No warranty

SIP Santral is provided **"as is"** under the [MIT License](LICENSE). There is no guarantee that it works without interruption or errors, or that it is fit for any particular purpose. The authors and contributors are not liable for any direct or indirect damage arising from the use of, or inability to use, the software, including missed calls, lost or unreadable recordings, service outages, loss of business, phone bills or data breaches.

### 2. Call recording and personal data

- SIP Santral records answered calls from start to end. Whoever installs and operates the system (the data controller) is solely responsible for recording and storing calls lawfully.
- **European Union:** GDPR requires a legal basis for recording (Art. 6) and informing callers (Art. 13).
- **Germany:** in addition, recording a conversation without consent can be a criminal offence under §201 StGB.
- **Turkey:** the Personal Data Protection Law No. 6698 (KVKK) requires informing callers, a legal basis for recording and keeping data no longer than necessary.
- **Other countries:** some jurisdictions require consent from one or all parties to a conversation.
- Your employees' conversations are recorded too. Follow employment law and employee information rules (e.g. works council approval in Germany).
- Recommendations: announce the recording in your greeting, keep the retention period as short as possible, limit access to the recordings folder, and switch recording off on lines that do not need it.

### 3. Emergency calls

SIP Santral makes no outgoing calls and **cannot** be used to reach 112, 911 or any other emergency number. Incoming calls cannot be answered during NAS, power, internet or provider outages either. Keep a backup solution for critical lines.

### 4. Security

- Securing the NAS, the network, ports and passwords is the operator's responsibility. Keep ADM up to date, use strong passwords, do not expose the panel (9490) or SIP (5060) ports to the internet, and use a VPN for remote access.
- SIP account credentials are stored on the NAS. The settings backup contains SIP and extension passwords in plain text.
- No liability is accepted for charges caused by misconfiguration or unauthorised access (e.g. abuse of your SIP account).

### 5. Third-party software and services

- Third-party components inside the APK are subject to their own licenses; see [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
- Piper voice models are downloaded from Hugging Face at the user's request. Each model has its own license; some restrict commercial use.
- The terms of your SIP provider, ASUSTOR (ADM, App Central), the STUN servers (Google, Cloudflare) and Hugging Face apply.

### 6. Trademarks and independence

This is an independent project. It is not affiliated with, sponsored or endorsed by ASUSTOR Inc., Sangoma Technologies or any other company mentioned. ASUSTOR, ADM and App Central are trademarks of ASUSTOR Inc.; Asterisk is a trademark of Sangoma Technologies. MicroSIP, Zoiper, Linphone and other names belong to their respective owners and are used only to describe compatibility.
