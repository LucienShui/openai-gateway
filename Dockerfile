FROM python:3.12-alpine
COPY ./requirements.txt /requirements.txt
RUN python3 -m pip install -r /requirements.txt
COPY ./ /app
WORKDIR /app
ENTRYPOINT ["python3", "main.py"]
