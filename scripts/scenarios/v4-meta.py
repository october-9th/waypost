# Kiểm tra dòng meta không bị cắt cụt.
typ("go scheduler", 0.03)
os.write(fd, b"\r")
pump(12.0)
snapshot("dong meta day du")
os.write(fd, b"q")
pump(1.0)
