FROM python:3.12-slim
COPY tests/fixture.py /qa/fixture.py
COPY tests/ehr_fixture.py /qa/ehr_fixture.py
USER 65534:65534
CMD ["python","-B","/qa/fixture.py"]
