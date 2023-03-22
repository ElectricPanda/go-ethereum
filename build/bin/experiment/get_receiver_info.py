import re
import statistics

max_request_size = 0
took_times = []

with open("/data/trieHashimoto/th_experiment_data_3/logs/0225_setHead_100000_[0:9]_pop.txt", "r") as file:
    for line in file:
        match = re.search(r'Took time: (\d+)', line)
        if match:
            took_times.append(int(match.group(1)))
        words = line.split()

        # 현재의 파일 형태에서는 words[7]을 넣어 queue의 max 사이즈를 볼 수 있습니다.
        request_size = int(words[10])

        if request_size > max_request_size:
            max_request_size = request_size

print(f"The maximum 'Request Size' value is: {max_request_size}")
# Calculate the average, standard deviation, and maximum
average = statistics.mean(took_times)
stdev = statistics.stdev(took_times)
maximum = max(took_times)

print('Average:', average)
print('Standard deviation:', stdev)
print('Maximum:', maximum)