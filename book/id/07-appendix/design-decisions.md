# Keputusan Teknis

Setiap keputusan arsitektural dalam proyek ini dibuat dengan trade-off tertentu. Dokumen ini menangkap alasan di balik setiap keputusan, alternatif yang dipertimbangkan, dan mengapa pendekatan yang dipilih menang.

---

## 1. Gin Gonic

**Keputusan:** Menggunakan Gin Gonic sebagai framework HTTP untuk semua layanan.

**Alasan:** Gin Gonic menawarkan keseimbangan terbaik antara performa dan ergonomi pengembang di antara framework HTTP Go. Gin mempertahankan throughput tinggi dengan overhead alokasi minimal (sebanding dengan `net/http` mentah di banyak benchmark) sambil menyediakan middleware chaining, request binding, dan pengelompokan rute secara bawaan. Ekosistem middleware memungkinkan cross-cutting concerns -- recovery, logging, rate limiting, CORS -- untuk dikomposisikan secara deklaratif daripada diduplikasi per handler. Alternatif seperti mux default `net/http` ditolak karena kurangnya routing berparameter dan dukungan middleware tanpa wrapper pihak ketiga; framework seperti Echo atau Fiber dipertimbangkan tetapi tidak menawarkan keunggulan berarti dibanding Gin untuk lingkup proyek ini. Menggunakan satu framework di semua 11 layanan memastikan pola yang konsisten untuk penanganan error, validasi permintaan, dan format respons, yang menyederhanakan orientasi dan mengurangi beban kognitif saat beralih antar layanan.

---

## 2. Penyimpanan Interface-First

**Keputusan:** Setiap layanan mendefinisikan interface `Storage` untuk lapisan datanya, dengan implementasi in-memory sebagai default.

**Alasan:** Mengabstraksikan penyimpanan di belakang interface berarti lapisan handler tidak pernah bergantung pada implementasi basis data tertentu. Default in-memory (didukung oleh map yang dilindungi `sync.RWMutex`) memungkinkan menjalankan setiap layanan dengan nol dependensi infrastruktur -- tanpa Redis, tanpa Postgres, tanpa basis data eksternal. Ketika deployment tingkat produksi diperlukan, implementasi baru dari interface yang sama (mis., `redisStorage` atau `postgresStorage`) dapat ditukar tanpa menyentuh satu handler pun. Pola ini juga menyederhanakan pengujian: suite tes menyuntikkan penyimpanan in-memory baru per kasus uji tanpa mengejek infrastruktur. Trade-off adalah lapisan indirection tipis dan sedikit lebih banyak kode per layanan, tetapi decoupling membenarkan biaya pada skala apa pun di luar prototipe sekali pakai.

---

## 3. `go.mod` Tunggal

**Keputusan:** Satu `go.mod` di root repositori -- tanpa workspace multi-modul.

**Alasan:** Repositori multi-modul menyelesaikan konflik versi ketika modul yang berbeda bergantung pada versi berbeda dari dependensi yang sama. Proyek ini tidak memiliki konflik seperti itu: semua 11 layanan dan 2 paket bersama menggunakan versi dependensi yang sama. `go.mod` tunggal berarti satu sumber kebenaran untuk resolusi dependensi, satu `go.sum`, dan tanpa konfigurasi workspace yang perlu dipertahankan. `go build ./...` dan `go test ./...` bekerja segera di seluruh pohon. Jika proyek ini berkembang hingga mencakup modul yang berversi independen (mis., SDK atau alat CLI terpisah), migrasi ke workspace multi-modul akan mudah -- tetapi untuk monorepo portofolio solo pada skala ini, modul tunggal adalah pilihan pragmatis.

---

## 4. Base62 Random Shortcode

**Keputusan:** Menghasilkan shortcode sebagai string Base62 7-karakter acak dengan collision retry, bukan melakukan hash pada URL.

**Alasan:** Alfabet Base62 7-karakter `[a-zA-Z0-9]` menghasilkan 62^7 = ~3,5 triliun kode unik, membuat collision sangat tidak mungkin pada skala praktis mana pun. Pendekatan pembuatan acak lebih sederhana daripada hashing URL: menghindari pemilihan fungsi hash, pemotongan agar sesuai dengan panjang kode, dan penanganan hash collision (yang masih membutuhkan logika retry). Ini juga menghilangkan kebutuhan untuk melakukan hash ulang ketika URL yang sama diperpendek dua kali -- setiap permintaan mendapatkan shortcode unik, yang sering kali merupakan perilaku yang diinginkan (misalnya, melacak sumber klik). Trade-off adalah bahwa URL identik menghasilkan shortcode berbeda, yang membuang penyimpanan dibandingkan dengan pendekatan yang dialamatkan berdasarkan konten, tetapi ini dapat diabaikan untuk jejak data URL shortener.

---

## 5. Sliding Window Rate Limiting

**Keputusan:** Mengimplementasikan rate limiting menggunakan algoritma sliding window.

**Alasan:** Sliding window rate limiting mengatasi masalah batas yang melekat pada algoritma fixed-window. Dengan fixed window, lonjakan permintaan di akhir satu jendela dan awal jendela berikutnya dapat menggandakan throughput yang diizinkan dalam interval pendek. Sliding window melacak timestamp permintaan dalam jendela saat ini secara fraksional, memperhalus transisi batas. Dibandingkan dengan token bucket -- kandidat umum lainnya -- sliding window menggunakan lebih sedikit memori untuk kunci dengan kardinalitas tinggi (tidak perlu mempertahankan hitungan token dan pengatur waktu isi ulang per kunci). Profil memori penting ketika melakukan rate limiting berdasarkan ID pengguna, IP, atau kunci API dalam sistem dengan jutaan klien unik. Trade-off adalah biaya CPU per-permintaan yang sedikit lebih tinggi untuk memangkas timestamp kedaluwarsa, tetapi ini dapat diabaikan relatif terhadap presisi yang diperoleh.

---

## 6. Virtual Nodes (150 Replica)

**Keputusan:** Menggunakan consistent hashing dengan 150 virtual node per physical node, dengan kunci checksum `crc32`.

**Alasan:** Consistent hashing memecahkan masalah re-sharding: ketika node bergabung atau pergi, hanya kunci K/N yang perlu dipindahkan (di mana K adalah total kunci dan N adalah jumlah node), dibandingkan dengan hampir semua kunci dalam skema hash-mod-N naif. Virtual node (juga disebut replica) mendistribusikan hash ring secara lebih seragam, mencegah hot spot ketika node memiliki kapasitas heterogen atau ketika sejumlah kecil node fisik akan menghasilkan pembagian yang tidak merata. Angka 150 replica adalah heuristik yang mapan dari makalah Dynamo Amazon -- cukup tinggi untuk memberikan keseragaman yang baik, cukup rendah untuk menjaga metadata ring tetap kecil. `crc32` dipilih daripada hash kriptografis (SHA-256, MD5) untuk kecepatan; ini tidak sensitif terhadap keamanan karena hash ring adalah struktur data internal. Trade-off adalah waktu pencarian O(log N) (binary search pada ring terurut) versus O(1) untuk hash-mod-N langsung, tetapi ring cukup kecil (150 * N entri) sehingga ini tidak relevan dalam praktik.

---

## 7. Snowflake 64-Bit IDs

**Keputusan:** Menghasilkan ID 64-bit unik menggunakan algoritma Snowflake: timestamp 41-bit + worker ID 10-bit + urutan 12-bit.

**Alasan:** ID Snowflake dapat diurutkan berdasarkan waktu, unik tanpa koordinasi, dan muat dalam integer 64-bit -- tipe native di Go dan sebagian besar basis data. Tata letak bit menghasilkan ~69 tahun ID dari epoch kustom, hingga 1024 worker, dan 4096 ID per milidetik per worker. Tidak diperlukan layanan eksternal (seperti urutan basis data atau ZooKeeper) untuk menghasilkannya. Ukuran 64-bit lebih kecil dari UUID (128 bit, representasi string 36 karakter), yang penting untuk ukuran indeks dan efisiensi penyimpanan. Trade-off adalah ketergantungan pada jam: jika jam worker mundur, ID dapat bertabrakan. Mitigasi standar termasuk memblokir hingga jam menyusul atau menggunakan epoch berbasis ZooKeeper. Untuk lingkup proyek ini, implementasi Snowflake dasar sudah cukup.

---

## 8. Trie-Based Autocomplete

**Keputusan:** Mengimplementasikan search autocomplete menggunakan trie in-memory dengan pengambilan frekuensi top-K.

**Alasan:** Trie menyediakan pencarian prefiks O(k) (di mana k adalah panjang prefiks), yang merupakan batas bawah teoretis untuk pencarian berbasis prefiks. Setiap node menyimpan daftar top-K pelengkapan yang diurutkan, dihitung sebelumnya dari data frekuensi, sehingga menanyakan "5 hasil teratas untuk prefiks 'ap'" hanya membutuhkan melintasi 'a' ke 'p' dan membaca daftar yang di-cache -- tanpa pengurutan atau peringkat pada waktu kueri. Ini membuatnya cocok untuk autocomplete yang sensitif terhadap latensi di mana setiap penekanan tombol memicu permintaan. Pembacaan konkuren aman melalui `sync.RWMutex`, memungkinkan banyak kueri paralel sementara tulisan memperoleh write lock selama re-indexing. Trade-off adalah memori: trie dengan node berbutir halus bisa menjadi besar. Varian trie kompak (radix tree, DAWG) dapat mengurangi memori dengan mengorbankan kompleksitas implementasi, tetapi untuk ukuran dataset yang ditargetkan proyek ini, trie sederhana sudah cukup.

---

## 9. Fan-Out on Write

**Keputusan:** Mendorong postingan baru ke timeline semua pengikut pada waktu tulis (fan-out on write).

**Alasan:** Fan-out on write (model push) menukar amplifikasi tulis dengan pembacaan timeline instan. Ketika pengguna memposting, sistem mengiterasi pengikut pengguna dan menyisipkan postingan ke dalam timeline setiap pengikut. Membaca timeline kemudian menjadi pencarian O(1) sederhana -- ambil daftar yang telah dihitung sebelumnya. Ini adalah pilihan yang tepat untuk pengguna dengan jutaan pengikut; untuk mereka, pendekatan hibrida (push ke pengikut aktif, pull dari tidak aktif) atau model pull murni lebih tepat. Untuk skala proyek ini, model push sederhana menunjukkan trade-off inti dengan jelas: baca cepat membutuhkan tulis mahal. Alternatif (fan-out on read / model pull) akan membutuhkan penggabungan timeline dari pengguna yang diikuti pada waktu kueri, menggeser biaya ke pembacaan.

---

## 10. BFS Crawler dengan Politeness

**Keputusan:** Mengimplementasikan web crawler sebagai traversal breadth-first melalui URL frontier berbasis channel, dengan rate limiting per-domain untuk kesopanan.

**Alasan:** BFS memastikan cakupan keluasan: crawler menemukan halaman lapis demi lapis daripada menyelam dalam ke satu domain. URL frontier diimplementasikan sebagai channel Go, menyediakan kontrol konkurensi alami -- worker mengonsumsi dari channel dan mengirimkan URL yang ditemukan kembali. Penundaan per-domain menegakkan kesopanan: setelah mengambil halaman dari `example.com`, crawler menunggu interval yang dapat dikonfigurasi sebelum mengambil dari domain yang sama. Ini menghormati arahan `robots.txt` dan mencegah membebani server asal mana pun. Ekstraksi tautan HTML menggunakan `golang.org/x/net/html`, pustaka Go standar untuk parsing HTML. Deduplikasi melalui Bloom filter atau set memastikan setiap URL di-crawl paling banyak sekali per proses. Trade-off adalah bahwa BFS dapat mengonsumsi memori yang signifikan untuk antrean frontier pada crawl besar, tetapi mengontrol kedalaman crawl mengurangi ini.

---

## 11. WebSocket Rooms

**Keputusan:** Mengimplementasikan ruang chat sebagai event loop berbasis goroutine dengan ring buffer untuk riwayat pesan.

**Alasan:** Setiap ruang chat adalah goroutine yang menjalankan event loop yang mendengarkan pada tiga channel: `join`, `leave`, dan `broadcast`. Model ini secara alami memetakan ke semantik chat WebSocket -- pengguna bergabung ke ruangan, mengirim pesan, dan menerima siaran. Pendekatan goroutine-per-ruangan efisien di Go: goroutine ringan (~2 KB stack) dan yang idle mengonsumsi sumber daya yang dapat diabaikan. Ring buffer dengan batas 100 pesan menyimpan riwayat terbaru, sehingga pengguna yang baru bergabung melihat pesan terbaru tanpa menanyakan basis data. Ring buffer berukuran tetap dan lock-free dalam goroutine tunggal, menghindari overhead sinkronisasi. Alternatif -- struktur data bersama dengan mutex untuk semua ruangan -- akan menggabungkan manajemen status ruangan dan mengurangi kejelasan. Trade-off adalah bahwa sejumlah besar ruangan idle dapat mengonsumsi overhead goroutine, tetapi deployment praktis jarang memiliki jutaan ruangan yang aktif secara bersamaan.

---

## 12. Multi-Channel Notification

**Keputusan:** Mengimplementasikan notifikasi melalui interface sender dengan implementasi terpisah untuk notifikasi in-app, email, dan push, dipisahkan melalui pub/sub.

**Alasan:** Interface sender (`Sender`) mendefinisikan satu kontrak: `Send(recipient, title, body)`. Setiap saluran mengimplementasikannya secara independen -- in-app (disimpan ke penyimpanan, diambil oleh klien), email (log ke stdout sebagai simulasi), dan push (log ke stdout sebagai simulasi). Publisher (layanan yang menghasilkan notifikasi) tidak pernah tahu tentang mekanisme pengiriman; mereka mempublikasikan ke saluran, dan sender terdaftar mengonsumsinya. Decoupling pub/sub ini berarti menambahkan saluran baru (SMS, Slack, webhook) membutuhkan nol perubahan pada publisher -- hanya menulis `Sender` baru dan mendaftarkannya. Trade-off adalah semantik pengiriman eventual: jika sender lambat atau gagal, saluran pub/sub harus menangani backpressure. Untuk proyek ini, saluran di-buffer dan sinkron, yang memadai untuk tujuan demonstrasi.

---

## 13. Simulasi Transcoding

**Keputusan:** Mensimulasikan transcoding video sebagai proses asinkron berbasis goroutine dengan state machine.

**Alasan:** Transcoding video mahal secara komputasi dan secara inheren asinkron. Implementasi ini memodelkan pipeline dunia nyata sebagai state machine: `uploading` ke `processing` ke `ready`. Ketika video diunggah, sebuah goroutine dibuat yang mensimulasikan pekerjaan transcoding (melalui `time.Sleep`), kemudian mentransisikan video ke `ready`. Ini mencerminkan pipeline transcoding produksi (AWS Elastic Transcoder, pekerjaan FFmpeg) tanpa memerlukan biner FFmpeg atau perangkat keras GPU yang sebenarnya. Pola state machine membuatnya mudah diperluas: menambahkan status `failed`, pelaporan kemajuan, atau varian kualitas paralel hanyalah status dan transisi tambahan. Trade-off adalah bahwa pekerjaan simulasi dapat diprediksi secara tidak realistis -- waktu transcoding nyata bervariasi dengan panjang video, resolusi, dan codec -- tetapi ini dapat diterima untuk mendemonstrasikan pola pemrosesan async dan desain API.

---

## 14. File Versioning

**Keputusan:** Menyimpan riwayat versi immutable untuk file -- setiap pembaruan menambahkan versi baru daripada menimpa di tempat.

**Alasan:** Versioning immutable berarti setiap pembaruan file membuat snapshot immutable baru dari konten file, dengan nomor versi yang bertambah. Versi lama dipertahankan dan dapat diakses oleh ID versi, memungkinkan rollback, jejak audit, dan akses konkuren ke status historis. Ini adalah desain yang sama yang digunakan oleh Google Drive, Dropbox, dan versioning objek S3. Implementasi menyimpan versi dalam irisan terurut per entri metadata file; versi terbaru selalu merupakan elemen terakhir. Trade-off adalah amplifikasi penyimpanan: mengedit file 100 MB sebanyak 50 kali mengonsumsi ~5 GB penyimpanan mentah. Sistem produksi menggunakan delta encoding atau penjadwalan snapshot untuk mengurangi ini, tetapi untuk proyek ini, penyimpanan versi penuh adalah demonstrasi konsep yang paling jelas dan dapat diterima pada skala data yang ditargetkan.

---

## 15. Graceful Shutdown

**Keputusan:** Setiap layanan menangani SIGINT dan SIGTERM dengan timeout graceful shutdown 5 detik.

**Alasan:** Layanan yang mati tanpa menguras permintaan yang sedang berlangsung dapat merusak data, menjatuhkan pesan, atau membuat klien menggantung. Pola graceful shutdown -- menangkap sinyal OS, memberi tahu server HTTP untuk berhenti menerima permintaan baru, menunggu permintaan aktif selesai (dengan timeout), lalu keluar -- memastikan pembersihan yang rapi. Timeout 5 detik adalah default yang wajar: cukup lama untuk menyelesaikan permintaan HTTP tipikal (yang seharusnya memakan waktu milidetik), cukup pendek sehingga proses tidak akan menggantung tanpa batas pada handler yang macet. Pola ini diimplementasikan sekali di `pkg/kit` dan digunakan kembali oleh setiap layanan, memastikan perilaku yang konsisten di seluruh proyek. Alternatif -- mengabaikan sinyal atau memanggil `os.Exit(0)` segera -- hanya sesuai untuk pekerjaan batch tanpa status, bukan layanan jaringan.
