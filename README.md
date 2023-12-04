# DowparGo
Partition Downloader dengan menggunakan bahasa GO

Program ini memungkinkan Anda mengunduh file dari URL yang diberikan, membaginya menjadi beberapa partisi, dan menggabungkannya kembali menjadi satu file.

## Penggunaan

Untuk menggunakan program ini, ikuti langkah-langkah di bawah ini:

1. Pastikan Anda memiliki lingkungan Go yang terinstal di sistem. [jika belum](https://go.dev/doc/install)

2. Unduh kode program:

    ```bash
    [git clone https://github.com/username/repository.git](https://github.com/yogaardiansyah/DowparGo.git)
    ```

3. Pindah ke direktori program:

    ```bash
    buka terminal lalu masuk ke direktori dengan :
    
    cd (lokasi_repository)
    ```

4. Jalankan program dengan perintah berikut:

    ```bash
    go run main.go -url <URL> -output <lokasi_direktori> [--keep-partition|--remove-partition]
    ```

    - `<URL>`: URL file yang akan diunduh.
    - `--keep-partition`: Opsional, biarkan file partisi setelah digabungkan.
    - `--remove-partition`: Opsional, hapus direktori partisi setelah digabungkan.
    - secara default keep-partition telah berjalan

Contoh penggunaan:

```bash
go run main.go -url https://static.wikia.nocookie.net/minecraft_gamepedia/images/a/a4/Bedrock_trading_interface.png -output C:/...(lokasi)/ --keep-partition
```
# Fitur
    - Mengunduh berkas dari URL yang ditentukan.
    - Menentukan otomatis apakah berkas perlu dibagi berdasarkan ukurannya.
    - Membagi berkas menjadi beberapa partisi dan mengunduhnya secara bersamaan.
    - Menggabungkan partisi menjadi berkas akhir.
    - Fitur opsional untuk menyimpan atau menghapus berkas partisi setelah penggabungan.

# Batas Partisi
Tools ini menggunakan ambang batas berikut untuk menentukan jumlah partisi:

    - Berkas kecil: < 10 KB (tidak dipartisi)
    - Berkas menengah: 10 KB - 500 KB (2 Partisi)
    - Berkas besar: 500 KB - 5 MB (5 Partisi)
    - Berkas sangat besar: > 5 MB (10 Partisi)
    
```
Dibuat Oleh Saya
```
