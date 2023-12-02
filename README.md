# DowparGo
Partition Downloader dengan menggunakan bahasa GO

Program ini memungkinkan Anda mengunduh file dari URL yang diberikan, membaginya menjadi beberapa partisi, dan menggabungkannya kembali menjadi satu file.

## Penggunaan

Untuk menggunakan program ini, ikuti langkah-langkah di bawah ini:

1. Pastikan Anda memiliki lingkungan Go yang terinstal di sistem.

2. Unduh kode program:

    ```bash
    git clone https://github.com/username/repository.git
    ```

3. Pindah ke direktori program:

    ```bash
    cd repository
    ```

4. Jalankan program dengan perintah berikut:

    ```bash
    go run main.go -url <URL> <lokasi_direktori> [--keep-partition|--remove-partition]
    ```

    - `<URL>`: URL file yang akan diunduh.
    - `--keep-partition`: Opsional, biarkan file partisi setelah digabungkan.
    - `--remove-partition`: Opsional, hapus direktori partisi setelah digabungkan.
    - secara general keep-partition telah berjalan

Contoh penggunaan:

```bash
go run main.go -url (https://example.com/largefile.zip) <lokasi_direktori> 
