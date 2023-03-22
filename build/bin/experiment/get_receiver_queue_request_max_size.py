max_request_size = 0

with open("/data/trieHashimoto/th_experiment_data_3/logs/0225_setHead_100000_[0:9]_pop.txt", "r") as file:
    for line in file:
        words = line.split()
        try:
            # 현재의 파일 형태에서는 words[7]을 넣어 queue의 max 사이즈를 볼 수 있습니다.
            request_size = int(words[10])
        except (IndexError, ValueError):
            # Skip over any rows that are not formatted correctly
            continue
        if request_size > max_request_size:
            max_request_size = request_size

print(f"The maximum 'Request Size' value is: {max_request_size}")