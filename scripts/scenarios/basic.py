# Tìm một topic đông bài, đi xuống vài dòng, thoát.
# Cố tình KHÔNG bấm enter/c/y: chúng mở browser thật và ghi đè clipboard thật.
typ("go scheduler", 0.03)
os.write(fd, b"\r")
pump(8.0)
snapshot("ket qua")
typ("jjj", 0.15)
pump(0.5)
snapshot("sau khi cuon")
os.write(fd, b"q")
pump(1.0)
